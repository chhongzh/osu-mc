package main

import (
	"log"
	"machine"
	"time"

	"tinygo.org/x/drivers/netdev"

	"osu-mc/driver/sh1106"
	"osu-mc/internal/animation"
	"osu-mc/internal/application"
	"osu-mc/internal/ui"
)

// These are not the pins the ESP32-S3 already uses: GPIO19/20 are USB
// D-/D+, GPIO45/46 and GPIO0/3 are strapping pins, GPIO26~32 connect to
// flash/PSRAM, and GPIO43/44 are UART0.
const (
	SCREEN_CLK_PIN = machine.GPIO4
	SCREEN_SDA_PIN = machine.GPIO5
	SCREEN_RES_PIN = machine.GPIO6
	SCREEN_DC_PIN  = machine.GPIO7
	SCREEN_CS_PIN  = machine.GPIO15

	KBD_UP_PIN    = machine.GPIO16
	KBD_DOWN_PIN  = machine.GPIO17
	KBD_LEFT_PIN  = machine.GPIO18
	KBD_RIGHT_PIN = machine.GPIO8
	KBD_MID_PIN   = machine.GPIO3
	KBD_SET_PIN   = machine.GPIO46
	KBD_RST_PIN   = machine.GPIO9
)

const (
	MAX_FPS                    = 120
	MIN_WAIT_TIME_PER_FRAME_MS = 1000 / MAX_FPS

	// With the screen off nothing is updated or drawn; the keys are only
	// sampled this often, to wake up. Two samples debounce a press, so a
	// press wakes the screen within about 100 ms.
	SLEEP_POLL_MS = 50

	// A frame longer than this is treated as this long, so a stall does not
	// make every animation jump to its end.
	MAX_FRAME_DT = 0.1

	screenWidth  = 128
	screenHeight = 64
)

var (
	buttons [6]*button
	btnRst  *button
)

func initKbds() {
	buttons = [...]*button{
		newButton(KBD_UP_PIN, ui.KeyUp, true),
		newButton(KBD_DOWN_PIN, ui.KeyDown, true),
		// Left and right repeat, so a held key scrolls all the way
		// across a row. At the start of a row, pressing left bubbles up
		// to the Viewer to go back, but repeats only reach the element
		// that took the press, so holding left does not pop several
		// pages at once.
		newButton(KBD_LEFT_PIN, ui.KeyLeft, true),
		newButton(KBD_RIGHT_PIN, ui.KeyRight, true),
		newButton(KBD_MID_PIN, ui.KeyEnter, false),
		newButton(KBD_SET_PIN, ui.KeyBack, false),
	}
	btnRst = newButton(KBD_RST_PIN, 0, false)
}

func main() {
	initKbds()

	if err := machine.SPI0.Configure(machine.SPIConfig{
		Frequency: 10_000_000,
		SCK:       SCREEN_CLK_PIN,
		SDO:       SCREEN_SDA_PIN,
	}); err != nil {
		log.Fatal("failed to configure SPI0: ", err)
	}

	dev := sh1106.New(machine.SPI0, SCREEN_DC_PIN, SCREEN_RES_PIN, SCREEN_CS_PIN)
	if err := dev.Configure(sh1106.Config{
		Width:  screenWidth,
		Height: screenHeight,
	}); err != nil {
		log.Fatal("failed to create display: ", err)
	}

	if err := dev.ClearDisplay(); err != nil {
		log.Fatal("failed to clear display: ", err)
	}

	// Key events are routed along the focus path by the Router: the
	// Viewer, then the top page, then the page's focus. The menus and
	// pages live in internal/application; here only the hardware is
	// wired up.
	viewer := ui.NewViewer(screenWidth)
	viewer.Height = screenHeight
	router := &ui.Router{Root: viewer}

	// The radio comes up only on the first scan or join; net/http
	// reaches it through the netdev.
	link := &wifiLink{}
	netdev.UseNetdev(link)
	power := &screenPower{}
	sys := &system{}

	app := application.New(viewer, application.Services{
		WiFi:   station{link},
		HTTP:   httpClient{},
		Power:  power,
		System: sys,
	})
	app.Start()

	run(dev, viewer, router, power, sys)
}

// screenPower is the Settings page's way to turn the screen off. It only
// raises a flag: ScreenOff is called while a key event is dispatched, and
// the loop turns the panel off once that frame is done.
type screenPower struct{ off bool }

func (p *screenPower) ScreenOff() { p.off = true }

func handleInput(r *ui.Router, v *ui.Viewer, dt float32) {
	for _, b := range buttons {
		if e, ok := b.poll(dt); ok && !b.swallow(e) {
			r.Dispatch(e)
		}
	}
	if e, ok := btnRst.poll(dt); ok && !btnRst.swallow(e) && e.Phase == ui.Release {
		v.PopToRoot()
	}
}

// run is the main loop: read the keys, update, and draw when something
// changed, at up to MAX_FPS. Sleeping for the rest of the frame, instead of
// a ticker, lets the loop drop to another rate while the screen is off.
func run(dev *sh1106.Device, root *ui.Viewer, r *ui.Router, power *screenPower, sys *system) {
	const frame = MIN_WAIT_TIME_PER_FRAME_MS * time.Millisecond

	canvas := ui.NewCanvas(dev)
	last := time.Now()

	for {
		if power.off {
			sleep(dev, r)
			power.off = false
			// The time spent asleep is not a frame: animations go on from
			// where they stopped, and the panel is redrawn in case the
			// controller lost what it showed.
			last = time.Now()
			animation.Default.Invalidate()
			sys.pause()
		}

		now := time.Now()
		dt := min(float32(now.Sub(last).Seconds()), MAX_FRAME_DT)
		last = now

		handleInput(r, root, dt)
		root.Update(dt)

		// Nothing moved and nothing was invalidated: the last frame is
		// still on the panel.
		drew := animation.Default.Update(dt)
		if drew {
			dev.Clear(sh1106.Off)
			root.Draw(canvas)
			dev.Display()
		}

		busy := time.Since(now)
		sys.frame(now, drew, busy)
		if rest := frame - busy; rest > 0 {
			time.Sleep(rest)
		}
	}
}

// sleep turns the panel off and waits for any key. Until then neither
// Update nor Draw runs, so the UI costs nothing but a key sample every
// SLEEP_POLL_MS; network work already started goes on in its goroutines.
//
// The key that wakes the screen does nothing else: its press, repeats and
// release are swallowed, so waking up does not also choose or go back.
func sleep(dev *sh1106.Device, r *ui.Router) {
	// A key held when the screen went off would otherwise send its Release
	// to an element that no longer expects it.
	r.Cancel()
	if err := dev.Sleep(true); err != nil {
		println("screen off failed:", err.Error())
		return
	}
	const dt = SLEEP_POLL_MS / 1000.0
	for {
		time.Sleep(SLEEP_POLL_MS * time.Millisecond)
		if wake(dt) {
			break
		}
	}
	if err := dev.Sleep(false); err != nil {
		println("screen on failed:", err.Error())
	}
}

// wake samples every key once and reports whether one went down. Each key
// that did is muted until it comes up.
func wake(dt float32) bool {
	woke := false
	for _, b := range buttons {
		woke = b.wake(dt) || woke
	}
	return btnRst.wake(dt) || woke
}
