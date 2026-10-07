package main

import (
	"machine"

	"osu-mc/internal/ui"
)

// Buttons are polled once per frame instead of handled in pin interrupts.
// At 120 fps a sample is taken every ~8 ms, which is well below any human
// press, and it keeps all UI state on one goroutine: an interrupt handler
// that writes animation targets races with the frame that reads them.
const (
	debounceSamples = 2    // consecutive samples a new state must hold
	repeatDelay     = 0.20 // seconds before a held button starts repeating
	repeatInterval  = 0.07 // seconds between the first repeats
	repeatFastest   = 0.03 // the interval shrinks down to this while held
	repeatAccel     = 0.9  // factor applied to the interval on every repeat
)

type button struct {
	pin    machine.Pin
	key    ui.Key
	repeat bool

	down     bool // debounced state
	pending  uint8
	held     float32
	next     float32
	interval float32
	repeated bool // repeated while held
	muted    bool // woke the screen; its events are dropped until it is up
}

func newButton(pin machine.Pin, key ui.Key, repeat bool) *button {
	pin.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	return &button{pin: pin, key: key, repeat: repeat}
}

// poll samples the button and returns what happened to it since the last
// frame, if anything.
//
// Going down is a Press and coming up a Release; the UI shows the press and
// acts on the release. A repeating button that is held past repeatDelay
// sends Repeats in between, faster and faster, and its Release then says
// so, so the action is not done once more.
func (b *button) poll(dt float32) (ui.Event, bool) {
	raw := b.pin.Get()
	if raw != b.down {
		b.pending++
		if b.pending < debounceSamples {
			return ui.Event{}, false
		}
		b.pending = 0
		b.down = raw
		if raw {
			b.held, b.next, b.interval = 0, repeatDelay, repeatInterval
			b.repeated = false
			return ui.Event{Key: b.key, Phase: ui.Press}, true
		}
		return ui.Event{Key: b.key, Phase: ui.Release, Repeated: b.repeated}, true
	}
	b.pending = 0

	if b.down && b.repeat {
		b.held += dt
		if b.held >= b.next {
			// Speed up the longer the button is held. The list springs
			// build up momentum from it, and coast when it is let go.
			b.interval = max(repeatFastest, b.interval*repeatAccel)
			b.next += b.interval
			b.repeated = true
			return ui.Event{Key: b.key, Phase: ui.Repeat}, true
		}
	}
	return ui.Event{}, false
}

// wake samples the button while the screen is off and reports whether it
// went down. A button that does is muted, so the rest of that press does not
// reach the UI.
func (b *button) wake(dt float32) bool {
	e, ok := b.poll(dt)
	if ok && e.Phase == ui.Press {
		b.muted = true
		return true
	}
	return false
}

// swallow reports whether e belongs to the press that woke the screen, and
// unmutes the button at its release.
func (b *button) swallow(e ui.Event) bool {
	if !b.muted {
		return false
	}
	if e.Phase == ui.Release {
		b.muted = false
	}
	return true
}
