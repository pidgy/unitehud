package ui

import (
	"image"
	"time"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/pidgy/unitehud/core/fonts"
	"github.com/pidgy/unitehud/core/notify"
	"github.com/pidgy/unitehud/gui/cursor"
	"github.com/pidgy/unitehud/gui/is"
	"github.com/pidgy/unitehud/gui/visual/decorate"
	"github.com/pidgy/unitehud/gui/visual/title"
	"github.com/pidgy/unitehud/media/img/splash"
	"github.com/pidgy/unitehud/media/video"
	"github.com/pidgy/unitehud/media/video/fps"
	"github.com/pidgy/unitehud/media/video/window/electron"
)

type client struct {
	hwnd uintptr

	video,
	overlay image.Image
	overlayOp paint.ImageOp

	bar *title.Widget

	windows struct {
		parent  *GUI
		current *app.Window
	}

	visibility struct {
		seen,
		hidden bool
	}

	dimensions struct {
		size image.Point

		maximized,
		fullscreened bool
	}

	hover,
	clicked time.Time
}

func (g *GUI) client(onclose func()) {
	ui := g.clientUI()

	ui.windows.current.Perform(system.ActionCenter)
	ui.windows.current.Perform(system.ActionRaise)

	err := electron.OpenWindow()
	if err != nil {
		notify.Warn("Client: Failed to render overlay (%v)", err)
	}
	defer electron.CloseWindow()

	defer fps.NewLoop(&fps.LoopOptions{
		Async: true,
		FPS:   120,
		Render: func(min, max, avg time.Duration) (close bool) {
			var err error

			img, err := video.Capture()
			if err != nil {
				g.ToastError(err)
				g.next(is.MainMenu)
				return true
			}

			ui.video = img
			ui.overlayOp = paint.NewImageOp(img)

			return false
		},
	}).Stop()

	defer fps.NewLoop(&fps.LoopOptions{
		Async: true,
		FPS:   1,
		Render: func(min, max, avg time.Duration) (close bool) {
			if ui.hwnd != 0 {
				go electron.Follow(ui.hwnd, ui.visibility.hidden)
			}
			return
		},
	}).Stop()

	defer onclose()

	var ops op.Ops

	for event := range ui.windows.current.Events() {
		switch e := event.(type) {
		case system.DestroyEvent:
			notify.System("Client: Closing...")
			return
		case system.StageEvent:
			if !ui.visibility.seen {
				ui.visibility.seen = true
			} else {
				ui.visibility.hidden = !ui.visibility.hidden
			}
		case app.ViewEvent:
			ui.hwnd = e.HWND
			ui.visibility.hidden = false
		case system.FrameEvent:
			gtx := layout.NewContext(&ops, e)

			if ui.dimensions.fullscreened {
				ui.bar.Hide = time.Since(ui.hover) > time.Second*2
			} else {
				ui.dimensions.size = e.Size
			}

			for _, e := range gtx.Events(g) {
				switch event := e.(type) {
				case key.Event:
					if event.State != key.Release {
						continue
					}

					switch event.Name {
					case key.NameF11:
						ui.fullscreen()
					case key.NameEscape:
						if ui.dimensions.fullscreened {
							ui.fullscreen()
						}
					default:
						if ui.dimensions.fullscreened {
							ui.bar.Hide = false
						}
					}
				case pointer.Event:
					switch event.Type {
					case pointer.Release:
						if time.Since(ui.clicked) < time.Second/2 {
							ui.fullscreen()
						}
						ui.clicked = time.Now()
					case pointer.Move, pointer.Enter:
						if !ui.dimensions.fullscreened {
							break
						}
						ui.hover = time.Now()
						ui.bar.Hide = false
					}
				}
			}

			fit := widget.Contain

			ui.bar.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return decorate.BackgroundAlt(gtx, func(gtx layout.Context) layout.Dimensions {
						layout.Flex{
							Axis: layout.Horizontal,
						}.Layout(
							gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {

									return widget.Image{
										Fit:      fit,
										Src:      ui.overlayOp,
										Position: layout.Center,
									}.Layout(gtx)
								})
							}),
						)

						layout.Flex{
							Axis: layout.Horizontal,
						}.Layout(
							gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									if ui.overlay != nil {
										return widget.Image{
											Fit:      fit,
											Src:      paint.NewImageOp(ui.overlay),
											Position: layout.Center,
										}.Layout(gtx)
									}
									return layout.Dimensions{Size: gtx.Constraints.Max}
								})
							}),
						)

						area := clip.Rect(gtx.Constraints).Push(gtx.Ops)

						pointer.InputOp{
							Tag:   g,
							Types: pointer.Enter | pointer.Move | pointer.Release,
						}.Add(gtx.Ops)

						key.InputOp{
							Tag:  g,
							Keys: key.Set(key.NameEscape),
						}.Add(gtx.Ops)

						area.Pop()

						return layout.Dimensions{Size: gtx.Constraints.Max}
					})
				},
			)
			if ui.bar.Hide {
				cursor.Is(pointer.CursorNone)
			}

			ui.windows.current.Invalidate()

			e.Frame(gtx.Ops)
		default:
			notify.Missed(event, "Client")
		}
	}
}

func (ui *client) fullscreen() {
	ui.dimensions.fullscreened = !ui.dimensions.fullscreened
	ui.bar.Hide = ui.dimensions.fullscreened

	if ui.dimensions.fullscreened {
		ui.windows.current.Option(app.Fullscreen.Option())
	} else {
		ui.windows.current.Perform(system.ActionUnmaximize)
		ui.windows.current.Option(app.Windowed.Option(), app.Size(unit.Dp(ui.dimensions.size.X), unit.Dp(ui.dimensions.size.Y)))
	}
}

func (g *GUI) clientUI() *client {
	ui := &client{
		video: splash.Projector(),
	}

	ui.bar = title.New(
		"UniteHUD",
		fonts.NewCollection(),
		func() { ui.windows.current.Perform(system.ActionMinimize) },
		ui.fullscreen,
		func() { ui.windows.current.Perform(system.ActionClose) },
	)
	ui.bar.NoDrag = false

	ui.dimensions.size = image.Pt(1280, 720)

	ui.windows.parent = g
	ui.windows.current = app.NewWindow(
		app.Title("UniteHUD Projector"),
		app.Size(unit.Dp(ui.dimensions.size.X), unit.Dp(ui.dimensions.size.Y)),
		app.MinSize(unit.Dp(ui.dimensions.size.X), unit.Dp(ui.dimensions.size.Y)),
		app.Decorated(false),
	)

	return ui
}