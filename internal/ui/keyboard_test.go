package ui

import (
	"testing"

	"osu-mc/internal/animation"
)

func newTestKeyboard() (*Keyboard, *Router) {
	k := NewKeyboard()
	k.Cursor = NewCursor()
	k.SetTarget(RectF{W: 128, H: 64})
	k.OnEnter(false)
	k.Cursor.Snap()
	return k, &Router{Root: k}
}

// findKey returns the (row, item) position of the first key in the
// keyboard's current layer matching want, or fails the test: it drives the
// "type a string" test below by locating targets the same way a human
// reading the layout would, rather than hard coding positions.
func findKey(t *testing.T, k *Keyboard, want func(d keyDef) bool) (row, item int) {
	t.Helper()
	for i, specs := range k.specs {
		for j, d := range specs {
			if want(d) {
				return i, j
			}
		}
	}
	t.Fatalf("no matching key in layer %d", k.layer)
	return 0, 0
}

// gotoKey drives the router with presses so the keyboard ends up with row
// rowIdx focused and item itemIdx selected within it, then leaves it
// selected without pressing enter. It presses left enough times to reach
// item 0 first (Row.Select clamps, so over-pressing is harmless) so the
// number of rights needed is always exactly itemIdx.
func gotoKey(t *testing.T, k *Keyboard, r *Router, rowIdx, itemIdx int) {
	t.Helper()
	for i := 0; i < 3; i++ {
		var cur int
		el := k.lines.Focused()
		for i, row := range k.rows {
			if Element(row) == el {
				cur = i
			}
		}
		if cur == rowIdx {
			break
		}
		key := KeyDown
		if rowIdx < cur {
			key = KeyUp
		}
		tapRouted(r, key)
	}
	row := k.rows[rowIdx]
	for i := 0; i < row.Len(); i++ {
		tapRouted(r, KeyLeft)
	}
	for i := 0; i < itemIdx; i++ {
		tapRouted(r, KeyRight)
	}
	if row.Selected() != itemIdx {
		t.Fatalf("selected %d after navigating, want %d", row.Selected(), itemIdx)
	}
}

// pressKey navigates to and activates the first key matching want.
func pressKey(t *testing.T, k *Keyboard, r *Router, want func(d keyDef) bool) {
	t.Helper()
	row, item := findKey(t, k, want)
	gotoKey(t, k, r, row, item)
	tapRouted(r, KeyEnter)
}

func isChar(r rune) func(d keyDef) bool {
	return func(d keyDef) bool { return d.kind == keyChar && d.r == r }
}

func TestKeyboardTypesMixedString(t *testing.T) {
	k, r := newTestKeyboard()

	// "Ab 1#!" exercises an upper and a lower letter, a space, a digit
	// and two symbols from two different non-letter layers, forcing
	// layer switches back and forth.
	pressKey(t, k, r, func(d keyDef) bool { return d.kind == keyCaps }) // caps on
	pressKey(t, k, r, isChar('a'))                                      // "A"
	pressKey(t, k, r, func(d keyDef) bool { return d.kind == keyCaps }) // caps off
	pressKey(t, k, r, isChar('b'))                                      // "b"
	pressKey(t, k, r, isChar(' '))                                      // " "
	pressKey(t, k, r, func(d keyDef) bool { return d.kind == keyLayer && d.layer == layerDigits })
	pressKey(t, k, r, isChar('1'))
	pressKey(t, k, r, func(d keyDef) bool { return d.kind == keyLayer && d.layer == layerSymbols })
	pressKey(t, k, r, isChar('#'))
	pressKey(t, k, r, func(d keyDef) bool { return d.kind == keyLayer && d.layer == layerDigits })
	pressKey(t, k, r, isChar('!'))

	if got, want := k.Text(), "Ab 1#!"; got != want {
		t.Fatalf("typed %q, want %q", got, want)
	}
}

// TestKeyboardCoversAllChars proves every visible ASCII character
// (0x20..0x7E) can be typed: the union of every layer's char keys, with
// letters counted in both cases since caps toggles in place, must be
// exactly that range.
func TestKeyboardCoversAllChars(t *testing.T) {
	got := map[rune]bool{}
	for layer, table := range [][3][]keyDef{keysLetters, keysDigits, keysSymbols} {
		for _, row := range table {
			for _, d := range row {
				if d.kind != keyChar {
					continue
				}
				got[d.r] = true
				if layer == layerLetters && d.r >= 'a' && d.r <= 'z' {
					got[d.r-32] = true
				}
			}
		}
	}
	for c := rune(0x20); c <= 0x7E; c++ {
		if !got[c] {
			t.Errorf("character %q (0x%02X) is not typeable", c, c)
		}
	}
	for c := range got {
		if c < 0x20 || c > 0x7E {
			t.Errorf("character %q (0x%02X) is outside the printable ASCII range", c, c)
		}
	}
}

func TestKeyboardBackspace(t *testing.T) {
	k, r := newTestKeyboard()
	k.SetText("abc")
	pressKey(t, k, r, func(d keyDef) bool { return d.kind == keyBackspace })
	if k.Text() != "ab" {
		t.Fatalf("text %q after backspace, want %q", k.Text(), "ab")
	}

	// Backspacing an empty field must not panic or go negative.
	k.SetText("")
	pressKey(t, k, r, func(d keyDef) bool { return d.kind == keyBackspace })
	if k.Text() != "" {
		t.Fatalf("text %q after backspace on empty, want empty", k.Text())
	}
}

func TestKeyboardMaxLen(t *testing.T) {
	k, r := newTestKeyboard()
	k.MaxLen = 3
	for i := 0; i < 5; i++ {
		pressKey(t, k, r, isChar('x'))
	}
	if k.Text() != "xxx" {
		t.Fatalf("text %q, want %q (capped at MaxLen)", k.Text(), "xxx")
	}
}

func TestKeyboardOnDoneFiresOnRelease(t *testing.T) {
	k, r := newTestKeyboard()
	k.SetText("hi")
	var got string
	calls := 0
	k.OnDone = func(s string) {
		got = s
		calls++
	}

	row, item := findKey(t, k, func(d keyDef) bool { return d.kind == keyDone })
	gotoKey(t, k, r, row, item)

	r.Dispatch(Event{Key: KeyEnter, Phase: Press})
	if calls != 0 {
		t.Fatalf("OnDone fired on press")
	}
	r.Dispatch(Event{Key: KeyEnter, Phase: Release})
	if calls != 1 || got != "hi" {
		t.Fatalf("OnDone called %d times with %q, want once with %q", calls, got, "hi")
	}
}

func TestKeyboardOnChange(t *testing.T) {
	k, r := newTestKeyboard()
	var got string
	k.OnChange = func(s string) { got = s }
	pressKey(t, k, r, isChar('x'))
	if got != "x" {
		t.Fatalf("OnChange saw %q, want %q", got, "x")
	}
}

// TestKeyboardRenderDoesNotPanic exercises typing, backspace, caps and
// layer switches, running long enough to see a caret blink, all while
// drawing to a screen that panics on any out-of-bounds pixel.
func TestKeyboardRenderDoesNotPanic(t *testing.T) {
	k, r := newTestKeyboard()
	run(k, 1)
	frame(k)

	pressKey(t, k, r, isChar('q'))
	pressKey(t, k, r, func(d keyDef) bool { return d.kind == keyCaps })
	pressKey(t, k, r, isChar('z'))
	pressKey(t, k, r, func(d keyDef) bool { return d.kind == keyLayer && d.layer == layerDigits })
	pressKey(t, k, r, func(d keyDef) bool { return d.kind == keyLayer && d.layer == layerSymbols })
	pressKey(t, k, r, func(d keyDef) bool { return d.kind == keyBackspace })

	for i := 0; i < 90; i++ {
		run(k, 1.0/120)
		frame(k)
	}
}

// TestKeyboardReleasesEverything pushes a keyboard into a Viewer, types a
// little, pops it and lets it fly out: once it has settled, every animated
// value it registered (field, rows and their items) must be gone.
func TestKeyboardReleasesEverything(t *testing.T) {
	v := NewViewer(128)
	v.Cursor = NewCursor()
	v.Push(newPage(v.Cursor, 3))
	run(v, 2)
	before := animation.Default.Len()

	k := NewKeyboard()
	k.Cursor = v.Cursor
	v.Push(k)
	run(v, 2)

	r := &Router{Root: v}
	pressKey(t, k, r, isChar('a'))
	pressKey(t, k, r, func(d keyDef) bool { return d.kind == keyLayer })

	tap(v, KeyBack)
	run(v, 2)

	if n := animation.Default.Len(); n != before {
		t.Fatalf("%d animated values registered after popping the keyboard, want %d", n, before)
	}
}

// TestRenderKeyboard prints a frame with go test -v, to eyeball the layout.
func TestRenderKeyboard(t *testing.T) {
	k, r := newTestKeyboard()
	k.SetText("hello")
	pressKey(t, k, r, isChar('q'))
	run(k, 1)
	k.Cursor.Snap()
	t.Log("\n" + frameWith(k, k.Cursor).String())
}

// Switching layers throws the old keys away; the cursor must end up on one
// of the new ones, in about the same place, not on a released item.
func TestKeyboardLayerKeepsCursor(t *testing.T) {
	k, r := newTestKeyboard()
	isLayer := func(d keyDef) bool { return d.kind == keyLayer }
	for i := 0; i < 2; i++ {
		row, item := findKey(t, k, isLayer)
		pressKey(t, k, r, isLayer)
		live := false
		for _, rw := range k.rows {
			for j := 0; j < rw.Len(); j++ {
				if k.Cursor.Target() == Focusable(rw.Item(j).(*Text)) {
					live = true
				}
			}
		}
		if !live {
			t.Fatalf("switch %d: the cursor is on a released key", i)
		}
		if got := k.rows[row].Selected(); got != min(item, k.rows[row].Len()-1) {
			t.Fatalf("switch %d: row %d selected %d, want %d kept", i, row, got, item)
		}
	}
}
