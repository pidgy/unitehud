//go:generate go-winres make --product-version=git-tag
package main

import (
	"os"
	"os/signal"

	"github.com/pidgy/unitehud/app"
	"github.com/pidgy/unitehud/avi/audio"
	"github.com/pidgy/unitehud/avi/video"
	"github.com/pidgy/unitehud/core/config"
	"github.com/pidgy/unitehud/core/detect"
	"github.com/pidgy/unitehud/core/notify"
	"github.com/pidgy/unitehud/core/server"
	"github.com/pidgy/unitehud/core/stats"
	"github.com/pidgy/unitehud/core/team"
	"github.com/pidgy/unitehud/gui/ui"
	"github.com/pidgy/unitehud/system/discord"
	"github.com/pidgy/unitehud/system/process"
	"github.com/pidgy/unitehud/system/save"
	"github.com/pidgy/unitehud/system/tray"
	"github.com/pidgy/unitehud/system/update"
)

var sigq = make(chan os.Signal, 1)

func init() {
	notify.Announce("[UniteHUD] Initializing...")
}

func signals() {
	signal.Notify(sigq, os.Interrupt)
	<-sigq

	notify.Announce("[UniteHUD] Closing...")

	video.Close()
	audio.Close()
	ui.Close()
	tray.Close()

	err := save.Logs(notify.FeedStrings(), stats.Lines(), stats.Counts())
	if err != nil {
		notify.Warn("[UniteHUD] Failed to save logs (%v)", err)
	}

	os.Exit(0)
}

func main() {
	defer ui.New().OnClose(func() { close(sigq) }).Open()

	err := process.Open()
	if err != nil {
		notify.Warn("[UniteHUD] Failed to stop previous process (%v)", err)
	}

	err = config.Open(config.Current.Gaming.Device)
	if err != nil {
		notify.Warn("[UniteHUD] Failed to load %s (%v)", config.Current.File(), err)
	}

	err = video.Open()
	if err != nil {
		notify.Warn("[UniteHUD] Failed to open video (%v)", err)
	}

	err = audio.Open()
	if err != nil {
		notify.Warn("[UniteHUD] Failed to open audio session (%v)", err)
	}

	err = server.Open()
	if err != nil {
		notify.Warn("[UniteHUD] Failed to start server (%v)", err)
	}

	err = tray.Open(app.Title, app.TitleVersion, ui.Close)
	if err != nil {
		notify.Warn("[UniteHUD] Failed to open system tray (%v)", err)
	}

	err = discord.Open()
	if err != nil {
		notify.Warn("[UniteHUD] Failed to open Discord RPC (%v)", err)
	}

	notify.Debug("[UniteHUD] Server Address (%s)", server.Address)
	notify.Debug("[UniteHUD] Recording (%t)", config.Current.Record)
	notify.Debug("[UniteHUD] Platform (%s)", config.Current.Gaming.Device)
	notify.Debug("[UniteHUD] Assets (%s)", config.Current.Assets())
	notify.Debug("[UniteHUD] Match Threshold: (%.0f%%)", config.Current.Acceptance*100)

	go detect.Clock()
	go detect.Energy()
	go detect.Preview()
	go detect.Defeated()
	go detect.Objectives()
	go detect.States()
	go detect.Scores(team.Self.Name)
	go detect.Scores(team.Purple.Name)
	go detect.Scores(team.Orange.Name)
	go detect.Scores(team.First.Name)
	go update.Check()

	go signals()

	notify.Announce("[UniteHUD] Initialized")
}
