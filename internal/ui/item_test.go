package ui

import (
	"testing"

	"osu-mc/internal/animation"
)

// textValues is how many animated values a Text registers: its frame and
// its scroll.
const textValues = 5

// An item inserted on screen opens up from nothing where it lands, while the
// selection stays on the item it was on, cursor and all.
func TestListInsertOpens(t *testing.T) {
	l := newTestList(5)
	l.Select(2)
	run(l, 1)
	sel := l.SelectedItem()
	above := l.Item(0).Frame().Target().Y

	n := NewText("new", nil)
	l.Insert(1, n)
	if l.Selected() != 3 || l.SelectedItem() != sel || l.Cursor.Target() != Focusable(sel.(*Text)) {
		t.Fatalf("insert moved the selection to %d", l.Selected())
	}
	l.Layout()
	if n.H.Current != 0 || n.H.Target != 16 {
		t.Fatalf("inserted item starts %v high, heading for %v, want 0 and 16", n.H.Current, n.H.Target)
	}
	// The list is centred: the selection stays where it is and the items
	// above it make room.
	if got := l.Item(0).Frame().Target().Y; got != above-18 {
		t.Fatalf("the first item is heading for %v, was %v, want one item higher", got, above)
	}
	run(l, 1)
	if !n.Settled() || n.H.Current != 16 {
		t.Fatalf("inserted item at %v high after a second", n.H.Current)
	}
}

// A removed item closes up and is released once it has; the selection and
// the cursor move to the item that takes its place.
func TestListRemoveSelected(t *testing.T) {
	l := newTestList(5)
	l.Select(2)
	run(l, 1)
	gone, next := l.Item(2).(*Text), l.Item(3)
	before := animation.Default.Len()

	l.Remove(2)
	if l.Len() != 4 || l.Selected() != 2 || l.SelectedItem() != next {
		t.Fatalf("after remove: %d items, selected %d", l.Len(), l.Selected())
	}
	if l.Cursor.Target() != Focusable(next.(*Text)) {
		t.Fatal("the cursor stayed on the removed item")
	}
	run(l, 0.05)
	if h := gone.H.Current; h <= 0 || h >= 16 {
		t.Fatalf("removed item is %v high, want it closing", h)
	}
	run(l, 2)
	if len(l.leaving) != 0 || animation.Default.Len() != before-textValues {
		t.Fatalf("removed item not released: %d leaving, %d values, want %d",
			len(l.leaving), animation.Default.Len(), before-textValues)
	}

	// At the end the selection falls back to the item before; with nothing
	// left the cursor hides.
	l.Select(3)
	l.Remove(3)
	if l.Selected() != 2 {
		t.Fatalf("removing the last item selected %d, want 2", l.Selected())
	}
	for l.Len() > 0 {
		l.Remove(0)
	}
	if l.Selected() != -1 || l.SelectedItem() != nil || l.Cursor.Target() != nil {
		t.Fatal("an empty list still has a selection")
	}
	l.Insert(0, NewText("back", nil))
	if l.Selected() != 0 {
		t.Fatal("an item put into an empty list was not selected")
	}
	l.Release()
}

// Static items are shown but the selection passes over them.
func TestListSkipsStatic(t *testing.T) {
	head, mid := NewText("Head", nil), NewText("Mid", nil)
	head.Static, mid.Static = true, true
	a, b := NewText("a", nil), NewText("b", nil)
	l := NewList(head, a, mid, b)
	l.Cursor = NewCursor()
	l.SetTarget(RectF{W: 128, H: 64})
	l.Snap()
	l.Focus()
	r := Router{Root: l}
	if l.SelectedItem() != ListItem(a) {
		t.Fatalf("selected %d, want the first item that is not static", l.Selected())
	}
	tapRouted(&r, KeyDown)
	if l.SelectedItem() != ListItem(b) || l.Cursor.Target() != Focusable(b) {
		t.Fatalf("down selected %d, want 3 past the static item", l.Selected())
	}
	tapRouted(&r, KeyUp)
	tapRouted(&r, KeyUp)
	if l.SelectedItem() != ListItem(a) {
		t.Fatalf("up selected %d, want 1, the heading is not selectable", l.Selected())
	}
	l.Release()
}

// SetItems swaps every item at once, keeping the place in the row and the
// cursor, and releases the old items right away.
func TestRowSetItems(t *testing.T) {
	c := NewCursor()
	row := newTestRow(c, 5)
	row.Snap()
	row.Focus()
	row.Select(4)
	items := []ListItem{NewText("x", nil), NewText("y", nil), NewText("z", nil)}
	before := animation.Default.Len()

	row.SetItems(items...)
	if row.Selected() != 2 || c.Target() != Focusable(items[2].(*Text)) {
		t.Fatalf("selected %d, want 2 with the cursor on it", row.Selected())
	}
	if animation.Default.Len() != before-5*textValues {
		t.Fatal("the old items were not released")
	}
	row.Layout()
	if f := items[0].Frame(); !f.Settled() || f.W.Current != 16 {
		t.Fatal("the new items did not appear in place")
	}
	row.Release()
}
