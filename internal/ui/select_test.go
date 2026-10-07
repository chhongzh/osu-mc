package ui

import "testing"

// newTestChoice returns a viewer with a list whose first line is a select,
// the way a settings page offers a choice.
func newTestChoice() (*Viewer, *Router, *Select, *List) {
	v := NewViewer(128)
	v.Cursor = NewCursor()
	sel := NewSelect(v, "Skin", "Default", "Rafis", "Whitecat")
	sel.Cursor = v.Cursor
	l := newPage(v.Cursor, 3)
	l.Insert(0, sel)
	l.Select(0)
	v.Push(l)
	run(v, 1)
	return v, &Router{Root: v}, sel, l
}

func TestSelectShowsValue(t *testing.T) {
	s := NewSelect(nil, "Skin", "Default", "Rafis")
	if s.Index() != 0 || s.Value() != "Default" || s.Field().Text() != "Default" {
		t.Fatalf("a new select is %d %q showing %q", s.Index(), s.Value(), s.Field().Text())
	}
	s.Format = func(o string) string { return "Skin: " + o }
	s.SetIndex(1)
	if s.Value() != "Rafis" || s.Field().Text() != "Skin: Rafis" {
		t.Fatalf("value %q shown as %q", s.Value(), s.Field().Text())
	}
	// Out of range is clamped, not wrapped.
	s.SetIndex(9)
	if s.Index() != 1 {
		t.Fatalf("SetIndex(9) of two options chose %d", s.Index())
	}

	// With nothing to choose from the label shows in brackets.
	empty := NewSelect(nil, "Skin")
	if empty.Index() != -1 || empty.Value() != "" || empty.Field().Text() != "[Skin]" {
		t.Fatalf("an empty select is %d %q showing %q", empty.Index(), empty.Value(), empty.Field().Text())
	}
}

func TestSelectOpensAndCommits(t *testing.T) {
	v, r, s, l := newTestChoice()
	if l.Focused() != Element(s) || v.Cursor.Target() != Focusable(s) {
		t.Fatal("the select is not focused")
	}
	var got []int
	s.OnChange = func(i int) { got = append(got, i) }

	// The page opens on release, not on press.
	r.Dispatch(Event{Key: KeyEnter, Phase: Press})
	if v.Len() != 1 {
		t.Fatal("opened on press")
	}
	r.Dispatch(Event{Key: KeyEnter, Phase: Release})
	page, ok := v.Top().(*List)
	if !ok || page == l {
		t.Fatal("enter did not open a page of options")
	}
	run(v, 1)

	// The heading is skipped and the chosen option is selected under it.
	if page.Item(0).(*Text).Text() != "Skin" || page.Item(0).Selectable() {
		t.Fatal("the page is not headed by a static label")
	}
	if page.Selected() != 1 {
		t.Fatalf("the page opened on %d, want the chosen option at 1", page.Selected())
	}

	tapRouted(r, KeyDown)
	tapRouted(r, KeyEnter)
	if s.Index() != 1 || s.Value() != "Rafis" || s.Field().Text() != "Rafis" {
		t.Fatalf("after choosing, the select is %d %q showing %q", s.Index(), s.Value(), s.Field().Text())
	}
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("OnChange got %v, want [1]", got)
	}
	if st, _ := v.State(page); st != PageExiting || v.Top() != Element(l) {
		t.Fatal("the page was not popped")
	}
	run(v, 1)
	if v.Cursor.Target() != Focusable(s) {
		t.Fatal("the cursor did not come back to the select")
	}

	// Opening again starts on the value chosen last, and the page is a fresh
	// one: a Viewer releases the old one once it has flown out.
	tapRouted(r, KeyRight)
	again := v.Top().(*List)
	if again == page {
		t.Fatal("the released page was pushed again")
	}
	run(v, 1)
	if again.Selected() != 2 {
		t.Fatalf("reopened on %d, want the chosen option at 2", again.Selected())
	}
}

func TestSelectBackCancels(t *testing.T) {
	v, r, s, _ := newTestChoice()
	s.SetIndex(2)
	called := false
	s.OnChange = func(int) { called = true }
	tapRouted(r, KeyEnter)
	page := v.Top().(*List)
	run(v, 1)
	tapRouted(r, KeyDown)
	tapRouted(r, KeyBack)
	if v.Top() == Element(page) || s.Index() != 2 || called {
		t.Fatalf("back left the select at %d, OnChange %v", s.Index(), called)
	}
}

// With nothing to choose from there is no page to open.
func TestSelectEmptyDoesNotOpen(t *testing.T) {
	v := NewViewer(128)
	v.Cursor = NewCursor()
	s := NewSelect(v, "Skin")
	s.Cursor = v.Cursor
	l := newPage(v.Cursor, 2)
	l.Insert(0, s)
	l.Select(0)
	v.Push(l)
	run(v, 1)

	tapRouted(&Router{Root: v}, KeyEnter)
	if v.Len() != 1 {
		t.Fatal("an empty select opened a page")
	}
}

// SetOptions keeps the chosen index as far as the new options go.
func TestSelectSetOptions(t *testing.T) {
	s := NewSelect(nil, "Skin", "a", "b", "c")
	s.SetIndex(2)
	s.SetOptions("x", "y")
	if s.Index() != 1 || s.Value() != "y" {
		t.Fatalf("after SetOptions the select is %d %q", s.Index(), s.Value())
	}
	s.SetOptions()
	if s.Index() != -1 || s.Field().Text() != "[Skin]" {
		t.Fatalf("emptied select is %d showing %q", s.Index(), s.Field().Text())
	}
}

// The select is a line like the others: down leaves it, up comes back.
func TestSelectInList(t *testing.T) {
	v, r, s, l := newTestChoice()
	tapRouted(r, KeyDown)
	if l.Selected() != 1 || v.Cursor.Target() != Focusable(l.Item(1).(*Text)) {
		t.Fatalf("down went to %d", l.Selected())
	}
	tapRouted(r, KeyUp)
	if l.Focused() != Element(s) || v.Cursor.Target() != Focusable(s) {
		t.Fatal("up did not come back to the select")
	}
}

// TestRenderSelect prints a select above a list, then its page of options,
// with go test -v.
func TestRenderSelect(t *testing.T) {
	v, r, _, _ := newTestChoice()
	t.Log("\n" + frame(v).String()) // the viewer draws the cursor
	tapRouted(r, KeyEnter)
	run(v, 1)
	t.Log("\n" + frame(v).String())
}
