package ui

import "testing"

// newTestSearch returns a viewer with a list whose first line is an input,
// the way a search box sits on a page.
func newTestSearch() (*Viewer, *Router, *Input, *List) {
	v := NewViewer(128)
	v.Cursor = NewCursor()
	in := NewInput(v, "Search")
	in.Cursor = v.Cursor
	l := newPage(v.Cursor, 5)
	l.Insert(0, in)
	l.Select(0)
	v.Push(l)
	run(v, 1)
	return v, &Router{Root: v}, in, l
}

func TestInputPlaceholder(t *testing.T) {
	in := NewInput(nil, "Name")
	if got := in.Field().Text(); got != "[Name]" {
		t.Fatalf("empty input shows %q, want [Name]", got)
	}
	in.Format = func(s string) string { return "Name: " + s }
	in.SetText("abc")
	if in.Text() != "abc" || in.Field().Text() != "Name: abc" {
		t.Fatalf("value %q shown as %q", in.Text(), in.Field().Text())
	}
	in.SetText("")
	if got := in.Field().Text(); got != "[Name]" {
		t.Fatalf("cleared input shows %q", got)
	}
	in.MaxLen = 2
	in.SetText("abc")
	if in.Text() != "ab" {
		t.Fatalf("MaxLen 2 kept %q", in.Text())
	}
}

func TestInputTypesAndCommits(t *testing.T) {
	v, r, in, l := newTestSearch()
	if l.Focused() != Element(in) || v.Cursor.Target() != Focusable(in) {
		t.Fatal("the input is not focused")
	}
	var done []string
	in.OnDone = func(s string) { done = append(done, s) }

	// The keyboard opens on release, not on press.
	r.Dispatch(Event{Key: KeyEnter, Phase: Press})
	if v.Len() != 1 {
		t.Fatal("opened on press")
	}
	r.Dispatch(Event{Key: KeyEnter, Phase: Release})
	k, ok := v.Top().(*Keyboard)
	if !ok {
		t.Fatal("enter did not open a keyboard")
	}
	if k.field.Text() != "[Search]" {
		t.Fatalf("keyboard field shows %q, want the placeholder", k.field.Text())
	}
	run(v, 1)

	pressKey(t, k, r, isChar('h'))
	pressKey(t, k, r, isChar('i'))
	if in.Text() != "" {
		t.Fatal("the value changed before OK")
	}
	pressKey(t, k, r, func(d keyDef) bool { return d.kind == keyDone })
	if in.Text() != "hi" || in.Field().Text() != "hi" {
		t.Fatalf("after OK the input is %q showing %q", in.Text(), in.Field().Text())
	}
	if len(done) != 1 || done[0] != "hi" {
		t.Fatalf("OnDone got %v", done)
	}
	if st, _ := v.State(k); st != PageExiting || v.Top() != Element(l) {
		t.Fatal("the keyboard was not popped")
	}
	run(v, 1)
	if v.Cursor.Target() != Focusable(in) {
		t.Fatal("the cursor did not come back to the input")
	}

	// Editing again starts from the value.
	tapRouted(r, KeyRight)
	k = v.Top().(*Keyboard)
	if k.Text() != "hi" {
		t.Fatalf("keyboard opened with %q, want hi", k.Text())
	}
}

func TestInputBackCancels(t *testing.T) {
	v, r, in, _ := newTestSearch()
	in.SetText("old")
	called := false
	in.OnDone = func(string) { called = true }
	tapRouted(r, KeyEnter)
	k := v.Top().(*Keyboard)
	run(v, 1)
	pressKey(t, k, r, isChar('x'))
	tapRouted(r, KeyBack)
	if v.Top() == Element(k) || in.Text() != "old" || called {
		t.Fatalf("back left the value %q, OnDone %v", in.Text(), called)
	}
}

// The input is a line like the others: down leaves it, up comes back.
func TestInputInList(t *testing.T) {
	v, r, in, l := newTestSearch()
	tapRouted(r, KeyDown)
	tapRouted(r, KeyDown)
	if l.Selected() != 2 || v.Cursor.Target() != Focusable(l.Item(2).(*Text)) {
		t.Fatalf("down went to %d", l.Selected())
	}
	tapRouted(r, KeyUp)
	tapRouted(r, KeyUp)
	if l.Focused() != Element(in) || v.Cursor.Target() != Focusable(in) {
		t.Fatal("up did not come back to the input")
	}
	tapRouted(r, KeyUp)
	if l.Selected() != 0 || v.Len() != 1 {
		t.Fatal("up at the input left the list")
	}
}

// TestRenderInput prints a search box above a list with go test -v.
func TestRenderInput(t *testing.T) {
	v, _, _, _ := newTestSearch()
	t.Log("\n" + frame(v).String()) // the viewer draws the cursor
}

// A value too long for the input scrolls while the input is focused, and
// goes back to its start once the focus leaves.
func TestInputScrollsWhenFocused(t *testing.T) {
	v, r, in, _ := newTestSearch()
	// The list indents the selected line by 4, the input's text too.
	if got := in.text.Target().X; got != 4 {
		t.Fatalf("text starts at %v, want 4 like the other lines", got)
	}
	in.SetText("a very long search query that cannot fit")
	if in.text.Overflow() == 0 {
		t.Fatal("test value fits")
	}
	run(v, 4)
	if in.text.Scroll() == 0 {
		t.Fatal("a focused long value did not scroll")
	}
	tapRouted(r, KeyDown)
	run(v, 1)
	if in.text.Scroll() > 0.5 || in.text.Marquee() {
		t.Fatalf("after the focus left the text is at %v", in.text.Scroll())
	}
}
