package ui

import "testing"

// newTestSettings returns a viewer with a list of checkboxes, the way a
// settings page is a list of switches.
func newTestSettings() (*Viewer, *Router, []*Checkbox, *List) {
	v := NewViewer(128)
	v.Cursor = NewCursor()
	boxes := []*Checkbox{NewCheckbox("Fullscreen"), NewCheckbox("Letterbox"), NewCheckbox("Hit sounds")}
	l := NewList()
	l.Cursor = v.Cursor
	l.ItemHeight = 16
	l.Offset = Stagger(4, 5)
	l.SetTarget(RectF{W: 128, H: 64})
	for _, b := range boxes {
		b.Cursor = v.Cursor
		l.Add(b)
	}
	v.Push(l)
	run(v, 1)
	return v, &Router{Root: v}, boxes, l
}

// Enter toggles on release, not on press, and OnChange follows the state.
func TestCheckboxToggles(t *testing.T) {
	v, r, boxes, _ := newTestSettings()
	b := boxes[0]
	if v.Cursor.Target() != Focusable(b) {
		t.Fatal("the first checkbox is not focused")
	}
	var got []bool
	b.OnChange = func(on bool) { got = append(got, on) }

	r.Dispatch(Event{Key: KeyEnter, Phase: Press})
	if b.Checked() {
		t.Fatal("toggled on press")
	}
	r.Dispatch(Event{Key: KeyEnter, Phase: Release})
	if !b.Checked() {
		t.Fatal("enter did not tick the box")
	}
	tapRouted(r, KeyEnter)
	if b.Checked() {
		t.Fatal("enter did not untick the box")
	}
	if len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("OnChange got %v, want [true false]", got)
	}
}

// SetChecked moves the mark without calling OnChange, and the mark grows and
// shrinks rather than appearing at once.
func TestCheckboxMarkAnimates(t *testing.T) {
	v, _, boxes, _ := newTestSettings()
	b := boxes[0]
	b.OnChange = func(bool) { t.Fatal("SetChecked called OnChange") }

	b.SetChecked(true)
	if b.fill.Current != 0 {
		t.Fatal("the mark appeared before it was stepped")
	}
	run(v, 1)
	if !b.fill.Settled() || b.fill.Current != 1 {
		t.Fatalf("the mark settled at %v, want 1", b.fill.Current)
	}

	b.SetChecked(false)
	run(v, 1)
	if b.fill.Current != 0 {
		t.Fatalf("the mark shrank to %v, want 0", b.fill.Current)
	}
}

// A ticked box draws a mark inside its outline; an unticked one draws only
// the outline.
func TestCheckboxDrawsMark(t *testing.T) {
	b := NewCheckbox("On")
	b.SetTarget(RectF{0, 0, 128, 16})
	b.Snap()

	empty := countOn(frame(b))
	b.SetChecked(true)
	b.Snap()
	ticked := countOn(frame(b))
	if ticked <= empty {
		t.Fatalf("a ticked box drew %d pixels, an empty one %d", ticked, empty)
	}

	// The mark stays inside the outline: the row through the middle of the
	// box has its two outline pixels on either side of the mark.
	s := frame(b)
	r := b.Rect()
	x := r.X + r.W - boxSize
	y := r.Y + (r.H-boxSize)/2
	if !s.at(x, y+boxSize/2) || !s.at(x+boxSize-1, y+boxSize/2) {
		t.Fatal("the mark swallowed the outline")
	}
}

// The checkbox is a line like the others: down leaves it, up comes back, and
// left is not its own, so it still goes back.
func TestCheckboxInList(t *testing.T) {
	v, r, boxes, l := newTestSettings()
	tapRouted(r, KeyDown)
	if l.Selected() != 1 || v.Cursor.Target() != Focusable(boxes[1]) {
		t.Fatalf("down went to %d", l.Selected())
	}
	tapRouted(r, KeyUp)
	if l.Focused() != Element(boxes[0]) {
		t.Fatal("up did not come back to the first box")
	}
	tapRouted(r, KeyLeft)
	if v.Len() != 1 {
		t.Fatal("left on a checkbox did not reach the viewer")
	}
}

// The cursor covers the label and the box together, not the label alone.
func TestCheckboxFocusCoversBox(t *testing.T) {
	_, _, boxes, _ := newTestSettings()
	b := boxes[0]
	f, t0 := b.FocusRect(), b.Target()
	if f != t0 {
		t.Fatalf("focus is %v, want the whole line %v", f, t0)
	}
	if b.label.Target().W >= t0.W {
		t.Fatal("the label was not given room for the box")
	}
}

// TestRenderCheckbox prints a settings page with go test -v.
func TestRenderCheckbox(t *testing.T) {
	v, _, boxes, _ := newTestSettings()
	boxes[0].SetChecked(true)
	boxes[2].SetChecked(true)
	run(v, 1)
	t.Log("\n" + frame(v).String()) // the viewer draws the cursor
}

func countOn(s *screen) int {
	n := 0
	for _, on := range s.px {
		if on {
			n++
		}
	}
	return n
}
