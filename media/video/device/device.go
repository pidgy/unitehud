package device

import (
	"fmt"
	"image"
	"sort"
	"strings"
	"time"

	"gocv.io/x/gocv"

	"github.com/pidgy/unitehud/core/config"
	"github.com/pidgy/unitehud/core/notify"
	"github.com/pidgy/unitehud/media/img"
	"github.com/pidgy/unitehud/media/img/splash"
	"github.com/pidgy/unitehud/media/video/device/win32"
	"github.com/pidgy/unitehud/media/video/monitor"
)

type device struct {
	id              int
	name            string
	closeq, closedq chan bool
	errq            chan error
}

var (
	active *device

	Sources, names = sources()

	mat = splash.DeviceMat().Clone()

	apis = struct {
		names  []string
		values map[string]int
	}{
		values: make(map[string]int),
	}
)

func init() {
	reset()

	go func() {
		for i := gocv.VideoCaptureAPI(1); i < 5000; i++ {
			api := i.String()
			if api == "" {
				continue
			}
			api = APIName(int(i))

			apis.values[api] = int(i)
			apis.names = append(apis.names, api)
		}
		sort.Strings(apis.names)

		// VideoCaptureAny should always be first.
		apis.names = append([]string{APIName(0)}, apis.names...)
	}()

	go func() {
		for ; ; time.Sleep(time.Second * 5) {
			s, n := sources()
			for _, got := range n {
				found := false
				for _, have := range names {
					if have == got {
						found = true
						break
					}
				}
				if !found {
					Sources, names = s, n
					notify.Debug("Device: Discovered \"%s\"", got)
					break
				}
			}
		}
	}()
}

func ActiveName() string {
	return active.name
}

func API(api string) int {
	if api == "" {
		return apis.values[apis.names[0]]
	}
	return apis.values[api]
}

func APIName(api int) string {
	return strings.Title(strings.ReplaceAll(gocv.VideoCaptureAPI(api).String(), "video-capture-", ""))
}

func APIs() []string {
	return apis.names
}

func Capture() (*image.RGBA, error) {
	return CaptureRect(monitor.MainResolution)
}

func CaptureRect(rect image.Rectangle) (*image.RGBA, error) {
	if mat.Empty() {
		return nil, nil
	}

	if !rect.In(monitor.MainResolution) {
		return nil, fmt.Errorf("illegal boundaries %s intersects %s", rect, monitor.MainResolution)
	}

	s := mat.Size()
	mrect := image.Rect(0, 0, s[1], s[0])

	if !rect.In(mrect) {
		return nil, fmt.Errorf("illegal boundaries %s, %s", rect, mrect)
	}

	return img.RGBA(mat.Region(rect))
}

func Close() {
	if active.id == config.NoVideoCaptureDevice {
		notify.Debug("Device: Ignorning call to close \"%s\" (inactive)", ActiveName())
		return
	}

	active.stop()

	notify.Debug("Device: Closed \"%s\"...", active.name)

	reset()
}

func (d *device) stop() {
	for t := time.NewTimer(time.Second * 5); ; {
		select {
		case d.closeq <- true:
		case <-d.closedq:
			if !t.Stop() {
				<-t.C
			}
			return
		case <-t.C:
			notify.Error("Device: %s failed to stop", d.name)
		}
	}
}

func IsActive() bool {
	return active.id != config.NoVideoCaptureDevice
}

func Name(d int) string {
	if d == config.NoVideoCaptureDevice {
		return "Disabled"
	}
	if d != config.NoVideoCaptureDevice && len(names) > d {
		return names[d]
	}
	return fmt.Sprintf("Device: %d", d)
}

func Open() error {
	if config.Current.VideoCaptureDevice == config.NoVideoCaptureDevice {
		return nil
	}

	if active.id != config.NoVideoCaptureDevice {
		notify.Debug("Device: Ignorning call to open \"%s\" (active)", ActiveName())
		return nil
	}

	active = &device{
		id:      config.Current.VideoCaptureDevice,
		name:    Name(config.Current.VideoCaptureDevice),
		closeq:  make(chan bool),
		closedq: make(chan bool),
		errq:    make(chan error),
	}

	go active.capture()

	err := <-active.errq
	if err != nil {
		reset()
		return err
	}

	return nil
}

func (d *device) capture() {
	defer close(d.closedq)

	api := APIName(API(config.Current.VideoCaptureAPI))

	notify.System("Device: Opening \"%s\" with API \"%s\"", d.name, api)
	defer notify.System("Device: Closing \"%s\"...", d.name)

	device, err := gocv.OpenVideoCaptureWithAPI(config.Current.VideoCaptureDevice, gocv.VideoCaptureAPI(API(config.Current.VideoCaptureAPI)))
	if err != nil {
		d.errq <- fmt.Errorf("%s does not support %s encoding", d.name, api)
		return
	}
	defer device.Close()

	notify.System("Device: Applying dimensions (%s)", monitor.MainResolution)

	device.Set(gocv.VideoCaptureFrameWidth, float64(monitor.MainResolution.Dx()))
	device.Set(gocv.VideoCaptureFrameHeight, float64(monitor.MainResolution.Dy()))
	capture := image.Rect(0, 0,
		int(device.Get(gocv.VideoCaptureFrameWidth)),
		int(device.Get(gocv.VideoCaptureFrameHeight)),
	)
	if !capture.Eq(monitor.MainResolution) {
		d.errq <- fmt.Errorf("%s has illegal dimensions %s", d.name, monitor.MainResolution)
		return
	}

	area := image.Rect(0, 0, int(device.Get(gocv.VideoCaptureFrameWidth)), int(device.Get(gocv.VideoCaptureFrameHeight)))
	if !area.Eq(monitor.MainResolution) {
		mat = splash.DeviceMat().Clone()
		d.errq <- fmt.Errorf("%s has invalid dimensions %s", d.name, area.String())
		return
	}

	close(d.errq)

	for d.running() {
		time.Sleep(time.Millisecond)

		if !device.Read(&mat) || mat.Empty() {
			notify.Warn("Device: Failed to capture from \"%s\"", d.name)
			return
		}
	}
}

func (d *device) running() bool {
	select {
	case <-d.closeq:
		return false
	default:
		return true
	}
}

func reset() {
	config.Current.VideoCaptureWindow = config.MainDisplay
	config.Current.VideoCaptureDevice = config.NoVideoCaptureDevice

	active = &device{
		id:      config.NoVideoCaptureDevice,
		name:    "Disabled",
		closeq:  make(chan bool),
		closedq: make(chan bool),
		errq:    make(chan error),
	}
}

func sources() ([]int, []string) {
	s := []int{}
	n := []string{}

	for i := 0; i < 10; i++ {
		name := win32.VideoCaptureDeviceName(i)
		if name == "" {
			break
		}

		s = append(s, i)
		n = append(n, name)
	}

	return s, n
}