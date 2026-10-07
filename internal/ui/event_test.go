package ui

import (
	"fmt"
	"testing"
)

// probe records the events it gets and claims presses of the keys in take.
type probe struct {
	Group
	name      string
	take      Key
	intercept Key
	child     Element
	log       *[]string
}

func (p *probe) Focused() Element { return p.child }

func (p *probe) HandleEvent(e Event) bool {
	*p.log = append(*p.log, fmt.Sprintf("%s:%d:%d", p.name, e.Key, e.Phase))
	return e.Key == p.take
}

func (p *probe) InterceptEvent(e Event) bool {
	if e.Key != p.intercept {
		return false
	}
	*p.log = append(*p.log, fmt.Sprintf("%s!%d:%d", p.name, e.Key, e.Phase))
	return true
}

func TestRouterBubblesAndCaptures(t *testing.T) {
	var log []string
	leaf := &probe{name: "leaf", take: KeyEnter, log: &log}
	mid := &probe{name: "mid", take: KeyLeft, child: leaf, log: &log}
	root := &probe{name: "root", intercept: KeyBack, child: mid, log: &log}
	r := Router{Root: root}

	check := func(want ...string) {
		t.Helper()
		if fmt.Sprint(log) != fmt.Sprint(want) {
			t.Fatalf("got %v, want %v", log, want)
		}
		log = log[:0]
	}

	// The leaf takes enter, and keeps it until the release.
	r.Dispatch(Event{Key: KeyEnter, Phase: Press})
	r.Dispatch(Event{Key: KeyEnter, Phase: Repeat})
	r.Dispatch(Event{Key: KeyEnter, Phase: Release})
	check("leaf:5:1", "leaf:5:2", "leaf:5:3")

	// Left bubbles past the leaf to the middle.
	r.Dispatch(Event{Key: KeyLeft, Phase: Press})
	r.Dispatch(Event{Key: KeyLeft, Phase: Release})
	check("leaf:3:1", "mid:3:1", "mid:3:3")

	// Back is intercepted at the root; nobody below sees it.
	r.Dispatch(Event{Key: KeyBack, Phase: Press})
	r.Dispatch(Event{Key: KeyBack, Phase: Release})
	check("root!6:1", "root!6:3")

	// The focus moves while left is down: the release still goes to
	// whoever took the press, not to the new focus.
	r.Dispatch(Event{Key: KeyLeft, Phase: Press})
	mid.child = &probe{name: "new", take: KeyLeft, log: &log}
	r.Dispatch(Event{Key: KeyLeft, Phase: Release})
	check("leaf:3:1", "mid:3:1", "mid:3:3")

	// Nobody takes up: its release goes nowhere.
	r.Dispatch(Event{Key: KeyUp, Phase: Press})
	if r.Dispatch(Event{Key: KeyUp, Phase: Release}) {
		t.Fatal("release of an unclaimed press was delivered")
	}
}

// Actions happen on release; the press only shows that they are coming.
func TestPressThenRelease(t *testing.T) {
	c := NewCursor()
	l := newTestList(5)
	l.Cursor = c
	l.Snap()
	l.Focus()
	c.Snap()
	selected := -1
	l.OnSelect = func(i int) { selected = i }
	r := Router{Root: l}

	rest := c.Rect()
	r.Dispatch(Event{Key: KeyDown, Phase: Press})
	run(l, 0.2)
	c.Update(0)
	run(c, 0.2)
	if l.Selected() != 0 {
		t.Fatal("moved on press")
	}
	if got := c.Rect(); got.Y <= rest.Y {
		t.Fatalf("cursor at %+v while down is held, want it leaning down from %+v", got, rest)
	}
	r.Dispatch(Event{Key: KeyDown, Phase: Release})
	if l.Selected() != 1 {
		t.Fatal("did not move on release")
	}

	// Held: every repeat moves, the release after them does not.
	r.Dispatch(Event{Key: KeyDown, Phase: Press})
	for i := 0; i < 2; i++ {
		r.Dispatch(Event{Key: KeyDown, Phase: Repeat})
	}
	r.Dispatch(Event{Key: KeyDown, Phase: Release, Repeated: true})
	if l.Selected() != 3 {
		t.Fatalf("selected %d after two repeats from 1, want 3", l.Selected())
	}

	// A page action fires once per press, repeats or not.
	rights := 0
	l.OnRight = func() { rights++ }
	r.Dispatch(Event{Key: KeyRight, Phase: Press})
	r.Dispatch(Event{Key: KeyRight, Phase: Repeat})
	r.Dispatch(Event{Key: KeyRight, Phase: Repeat})
	r.Dispatch(Event{Key: KeyRight, Phase: Release, Repeated: true})
	if rights != 1 {
		t.Fatalf("OnRight fired %d times for one held press", rights)
	}

	r.Dispatch(Event{Key: KeyEnter, Phase: Press})
	if !c.Pressed() || selected != -1 {
		t.Fatalf("enter press: pressed %v, selected %d", c.Pressed(), selected)
	}
	r.Dispatch(Event{Key: KeyEnter, Phase: Release})
	if c.Pressed() || selected != 3 {
		t.Fatalf("enter release: pressed %v, selected %d", c.Pressed(), selected)
	}
}

func newTestRow(c *Cursor, n int) *Row {
	r := NewRow()
	r.Cursor = c
	for i := 0; i < n; i++ {
		r.Add(NewText(fmt.Sprintf("%c", 'A'+i), nil))
	}
	r.ItemWidth = 16
	r.SetTarget(RectF{W: 128, H: 16})
	return r
}

func TestRowScrollsCentered(t *testing.T) {
	c := NewCursor()
	row := newTestRow(c, 20) // 20*18-2 = 358 px of content in 128
	row.Snap()
	row.Focus()
	r := Router{Root: row}
	x := func(i int) float32 { return row.Item(i).Frame().X.Target }

	if x(0) != 0 {
		t.Fatalf("first item at %v, want 0", x(0))
	}
	for i := 0; i < 8; i++ {
		tapRouted(&r, KeyRight)
	}
	run(row, 1)
	if got := x(8) + 8; got != 64 {
		t.Fatalf("item 8 centred at %v, want 64", got)
	}
	if c.Target() != Focusable(row.Item(8).(*Text)) {
		t.Fatal("cursor did not follow the row")
	}
	for i := 0; i < 30; i++ {
		tapRouted(&r, KeyRight)
	}
	row.Layout()
	if row.Selected() != 19 || x(19)+16 != 128 {
		t.Fatalf("last item %d ends at %v, want 19 at 128", row.Selected(), x(19)+16)
	}
}

// Left at the start of a row is not the row's: it goes back. Anywhere else
// the row keeps it.
func TestRowPassesLeftToViewer(t *testing.T) {
	v := NewViewer(128)
	v.Cursor = NewCursor()
	v.Push(newPage(v.Cursor, 3))
	run(v, 1)
	row := newTestRow(v.Cursor, 5)
	v.Push(row)
	run(v, 1)
	row.Select(2)
	r := Router{Root: v}

	tapRouted(&r, KeyLeft)
	tapRouted(&r, KeyLeft)
	if row.Selected() != 0 || v.Top() != Element(row) {
		t.Fatalf("selected %d, row on top %v", row.Selected(), v.Top() == Element(row))
	}

	// Press only peeks, release pops.
	r.Dispatch(Event{Key: KeyLeft, Phase: Press})
	run(v, 0.3)
	if st, _ := v.State(row); st != PageActive || v.stack[1].x.Current < 4 {
		t.Fatalf("while held: %v at x %v, want active and peeking", st, v.stack[1].x.Current)
	}
	r.Dispatch(Event{Key: KeyLeft, Phase: Release})
	if st, _ := v.State(row); st != PageExiting {
		t.Fatalf("after release the row is %v, want exiting", st)
	}
}

// newTestLines returns a list of lines, the way a page of rows is built,
// with no indent so that rows span the full width.
func newTestLines(c *Cursor, items ...ListItem) *List {
	l := NewList(items...)
	l.Cursor = c
	l.Gap = 0
	l.SetTarget(RectF{W: 128, H: 64})
	return l
}

func TestListOfRowsMovesFocusStraight(t *testing.T) {
	c := NewCursor()
	title := NewText("Title", nil)
	title.Static = true
	top := newTestRow(c, 8)
	top.ItemWidth = 12
	bottom := newTestRow(c, 8)
	bottom.ItemWidth = 30
	col := newTestLines(c, title, top, bottom)
	col.OnEnter(false)
	c.Snap()
	r := Router{Root: col}

	if col.Focused() != Element(top) {
		t.Fatal("the title took the focus")
	}
	for i := 0; i < 6; i++ {
		tapRouted(&r, KeyRight)
	}
	run(col, 1)
	tapRouted(&r, KeyDown)
	if col.Focused() != Element(bottom) || c.Target() != Focusable(bottom.SelectedItem().(*Text)) {
		t.Fatal("down did not move the focus to the next row")
	}
	// Item 6 of the top row is centred at 90; the nearest of the bottom
	// row, 32 px apart, is item 2 at 79.
	if bottom.Selected() != 2 {
		t.Fatalf("bottom row selected %d, want 2 below the top row's selection", bottom.Selected())
	}

	// Left and right now go to the bottom row, up past the top row
	// stays in the list: the title cannot be selected.
	tapRouted(&r, KeyRight)
	if bottom.Selected() != 3 || top.Selected() != 6 {
		t.Fatalf("right went to the wrong row: top %d bottom %d", top.Selected(), bottom.Selected())
	}
	tapRouted(&r, KeyUp)
	tapRouted(&r, KeyUp)
	if col.Focused() != Element(top) {
		t.Fatal("up did not come back to the top row")
	}
}

func tapRouted(r *Router, k Key) {
	r.Dispatch(Event{Key: k, Phase: Press})
	r.Dispatch(Event{Key: k, Phase: Release})
}

// TestRenderRows prints a list of rows with go test -v.
func TestRenderRows(t *testing.T) {
	c := NewCursor()
	title := NewText("Options", nil)
	title.Center = true
	title.Static = true
	mods := NewRow(NewText("EZ", nil), NewText("NF", nil), NewText("+HD", nil), NewText("HR", nil),
		NewText("DT", nil), NewText("FL", nil), NewText("SD", nil), NewText("PF", nil), NewText("SO", nil))
	mods.Cursor = c
	acts := NewRow(NewText("Play", nil), NewText("Autoplay", nil), NewText("Leaderboard", nil))
	acts.Cursor = c
	col := newTestLines(c, title, mods, acts)
	col.OnEnter(false)
	r := Router{Root: col}
	for i := 0; i < 4; i++ {
		tapRouted(&r, KeyRight)
	}
	run(col, 1)
	c.Snap()
	t.Log("\n" + frameWith(col, c).String())
}

func frameWith(e Element, c *Cursor) *screen {
	s := newScreen(128, 64)
	cv := NewCanvas(s)
	e.Draw(cv)
	c.Draw(cv)
	return s
}
