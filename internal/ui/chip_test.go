package ui

import "testing"

// newTestOptions returns a viewer with the options page: a heading, a row of
// mods that are each on or off, and a row of rates of which one is on.
func newTestOptions() (*Viewer, *Router, *Toggles, *Choice, *List) {
	v := NewViewer(128)
	v.Cursor = NewCursor()

	title := NewText("Options", nil)
	title.Static = true
	mods := NewToggles("EZ", "HR", "DT", "HD", "FL")
	rates := NewChoice("0.5x", "1.0x", "1.5x", "2.0x")
	for _, r := range []*Row{mods.Row, rates.Row} {
		r.Cursor = v.Cursor
	}

	l := NewList(title, mods, rates)
	l.Cursor = v.Cursor
	l.ItemHeight = RowHeight
	l.Gap = 0
	l.Marquee = false
	l.SetTarget(RectF{W: 128, H: 64})
	v.Push(l)
	run(v, 1)
	return v, &Router{Root: v}, mods, rates, l
}

// Enter switches the mod under the cursor on release, not on press, and the
// mods are independent: HD and DT are both in.
func TestTogglesSwitchIndependently(t *testing.T) {
	v, r, mods, _, l := newTestOptions()
	if l.Selected() != 1 || v.Cursor.Target() != Focusable(mods.Chip(0)) {
		t.Fatalf("the mods row is not focused, the list is on %d", l.Selected())
	}
	type change struct {
		i  int
		on bool
	}
	var got []change
	mods.OnChange = func(i int, on bool) { got = append(got, change{i, on}) }

	r.Dispatch(Event{Key: KeyEnter, Phase: Press})
	if mods.On(0) {
		t.Fatal("switched on press")
	}
	r.Dispatch(Event{Key: KeyEnter, Phase: Release})
	if !mods.On(0) {
		t.Fatal("enter did not switch the mod on")
	}

	// Walk to DT and HD and switch both: neither turns the other off.
	tapRouted(r, KeyRight)
	tapRouted(r, KeyRight)
	tapRouted(r, KeyEnter)
	tapRouted(r, KeyRight)
	tapRouted(r, KeyEnter)
	if !mods.On(2) || !mods.On(3) || !mods.On(0) {
		t.Fatal("switching a mod on turned another one off")
	}
	if mods.On(1) || mods.On(4) {
		t.Fatal("a mod that was never touched came on")
	}

	// Enter again on HD switches it back off.
	tapRouted(r, KeyEnter)
	if mods.On(3) || !mods.On(2) {
		t.Fatal("enter did not switch the mod back off")
	}
	want := []change{{0, true}, {2, true}, {3, true}, {3, false}}
	if len(got) != len(want) {
		t.Fatalf("OnChange got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("OnChange got %v, want %v", got, want)
		}
	}
}

// Exactly one option of a Choice is on: choosing turns the old one off.
func TestChoiceKeepsOneOn(t *testing.T) {
	v, r, _, rates, l := newTestOptions()
	if rates.Chosen() != 0 || !rates.Chip(0).On() {
		t.Fatalf("a new choice is on %d", rates.Chosen())
	}
	var got []int
	rates.OnChange = func(i int) { got = append(got, i) }

	tapRouted(r, KeyDown)
	if l.Selected() != 2 || v.Cursor.Target() != Focusable(rates.Chip(0)) {
		t.Fatalf("down went to %d", l.Selected())
	}
	tapRouted(r, KeyRight)
	tapRouted(r, KeyRight)
	tapRouted(r, KeyEnter)
	if rates.Chosen() != 2 || !rates.Chip(2).On() || rates.Chip(0).On() {
		t.Fatalf("after choosing, on is %d and option 0 is %v", rates.Chosen(), rates.Chip(0).On())
	}

	// Enter on the option that is already on changes nothing and does not
	// call OnChange again.
	tapRouted(r, KeyEnter)
	if rates.Chosen() != 2 || len(got) != 1 || got[0] != 2 {
		t.Fatalf("OnChange got %v, want [2]", got)
	}

	// Choosing in code moves the bar without calling OnChange.
	rates.Choose(0)
	if !rates.Chip(0).On() || rates.Chip(2).On() || len(got) != 1 {
		t.Fatalf("Choose called OnChange %v", got)
	}
	// Out of range is clamped, not wrapped.
	rates.Choose(99)
	if rates.Chosen() != rates.Len()-1 {
		t.Fatalf("Choose(99) of %d options chose %d", rates.Len(), rates.Chosen())
	}
}

// The bar under a chip grows and shrinks rather than appearing at once, and
// it is drawn under the label without being clipped away.
func TestChipBarAnimates(t *testing.T) {
	v, _, mods, _, _ := newTestOptions()
	c := mods.Chip(0)

	empty := countOn(frame(v))
	mods.SetOn(0, true)
	if c.fill.Current != 0 {
		t.Fatal("the bar appeared before it was stepped")
	}
	run(v, 1)
	if !c.fill.Settled() || c.fill.Current != 1 {
		t.Fatalf("the bar settled at %v, want 1", c.fill.Current)
	}
	if on := countOn(frame(v)); on == empty {
		t.Fatal("a chip that is on drew no bar")
	}

	mods.SetOn(0, false)
	run(v, 1)
	if c.fill.Current != 0 {
		t.Fatalf("the bar shrank to %v, want 0", c.fill.Current)
	}
}

// The cursor hugs the word, not the padded box around it, so the bar lines up
// with the label.
func TestChipFocusHugsLabel(t *testing.T) {
	_, _, mods, _, _ := newTestOptions()
	c := mods.Chip(0)
	f, box := c.FocusRect(), c.Target()
	if f.W >= box.W {
		t.Fatalf("focus is %v wide, want narrower than the chip %v", f.W, box.W)
	}
	if f.W != float32(c.Label().ContentWidth()) {
		t.Fatalf("focus is %v wide, want the label's %d", f.W, c.Label().ContentWidth())
	}
}

// The rows are lines of the list: up and down move between them, passing over
// the heading, and left at the first chip goes back.
func TestChipRowsInList(t *testing.T) {
	v, r, mods, rates, l := newTestOptions()
	tapRouted(r, KeyUp)
	if l.Selected() != 1 {
		t.Fatal("up did not pass over the static heading")
	}
	tapRouted(r, KeyDown)
	if l.Selected() != 2 || v.Cursor.Target() != Focusable(rates.Chip(0)) {
		t.Fatalf("down went to %d", l.Selected())
	}
	tapRouted(r, KeyLeft)
	if v.Len() != 1 {
		t.Fatal("left at the first chip did not reach the viewer")
	}
	// Right walks along the row rather than leaving it.
	tapRouted(r, KeyUp)
	tapRouted(r, KeyRight)
	if mods.Selected() != 1 || l.Selected() != 1 {
		t.Fatalf("right went to chip %d of line %d", mods.Selected(), l.Selected())
	}
}

// TestRenderChips prints the options page with mods on with go test -v.
func TestRenderChips(t *testing.T) {
	v, _, mods, rates, _ := newTestOptions()
	mods.SetOn(2, true) // DT
	mods.SetOn(3, true) // HD
	rates.Choose(2)
	run(v, 1)
	t.Log("\n" + frame(v).String()) // the viewer draws the cursor
}
