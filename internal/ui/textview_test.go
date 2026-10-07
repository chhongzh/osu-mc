package ui

import (
	"testing"
)

func newTestView(s string, wrap bool) *TextView {
	v := NewTextView(s, nil, wrap)
	v.Cursor = nil
	v.SetTarget(RectF{W: 128, H: 64})
	v.Snap()
	return v
}

func lines(v *TextView) []string {
	out := make([]string, v.Lines())
	for i := range out {
		out[i] = v.Line(i)
	}
	return out
}

func TestTextViewRawBreaksOnNewlineOnly(t *testing.T) {
	long := "this line is far too long to fit on a screen that is 128 pixels wide"
	v := newTestView("one\n"+long+"\n\nlast", false)
	got := lines(v)
	want := []string{"one", long, "", "last"}
	if len(got) != len(want) {
		t.Fatalf("lines %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d is %q, want %q", i, got[i], want[i])
		}
	}
	if mx, _ := v.maxScroll(); mx <= 0 {
		t.Fatalf("a long raw line should scroll sideways, max %v", mx)
	}
}

func TestTextViewWrapFits(t *testing.T) {
	s := "the quick brown fox jumps over the lazy dog, then a supercalifragilisticexpialidociouslylongword\nand a second paragraph"
	v := newTestView(s, true)
	w := v.textWidth()
	for i, l := range v.lines {
		if l.width > w {
			t.Errorf("line %d %q is %d wide, more than %d", i, v.Line(i), l.width, w)
		}
	}
	// Breaking at spaces drops them, breaking words keeps every rune, and
	// the paragraph break survives.
	para := false
	for i, l := range lines(v) {
		if len(l) > 0 && (l[0] == ' ' || l[len(l)-1] == ' ') {
			t.Errorf("line %d %q keeps the space it broke at", i, l)
		}
		if len(l) >= 5 && l[:5] == "and a" {
			para = true
		}
	}
	if !para {
		t.Errorf("the second paragraph does not start a line: %q", lines(v))
	}
	if v.Lines() < 4 {
		t.Errorf("only %d lines", v.Lines())
	}
	if mx, _ := v.maxScroll(); mx != 0 {
		t.Errorf("wrapped text scrolls sideways by %v", mx)
	}
}

func TestTextViewWrapTinyWidth(t *testing.T) {
	v := NewTextView("abc def", nil, true)
	v.SetTarget(RectF{W: viewPad + viewReserve + 1, H: 64})
	v.Snap()
	// Every line holds at least one rune, so this ends.
	if v.Lines() < 6 {
		t.Fatalf("got %d lines: %q", v.Lines(), lines(v))
	}
}

func TestTextViewScrollsAndClamps(t *testing.T) {
	s := "0\n1\n2\n3\n4\n5\n6\n7\n8\n9"
	v := newTestView(s, false)
	_, my := v.maxScroll()
	if my <= 0 {
		t.Fatalf("ten lines should not fit, max %v", my)
	}

	tap(v, KeyUp)
	if _, y := v.Scroll(); y != 0 {
		t.Fatalf("scrolled above the top to %v", y)
	}
	tap(v, KeyDown)
	if _, y := v.Scroll(); y != float32(v.LineHeight()) {
		t.Fatalf("down scrolled to %v, want one line", y)
	}
	for range 20 {
		tap(v, KeyDown)
	}
	if _, y := v.Scroll(); y != my {
		t.Fatalf("scrolled to %v, want the end %v", y, my)
	}
	run(v, 1)
	if v.scrollY.Current != my {
		t.Fatalf("settled at %v, want %v", v.scrollY.Current, my)
	}
}

func TestTextViewEdgePulls(t *testing.T) {
	v := newTestView("a\nb", false)
	r := Router{Root: v}
	r.Dispatch(Event{Key: KeyUp, Phase: Press})
	if v.pullY.Target == 0 {
		t.Fatal("holding up at the top did not pull")
	}
	run(v, 0.5)
	r.Dispatch(Event{Key: KeyUp, Phase: Release})
	if _, y := v.Scroll(); y != 0 {
		t.Fatalf("a blocked key scrolled to %v", y)
	}
	run(v, 1)
	if !v.pullY.Settled() || v.pullY.Current != 0 {
		t.Fatalf("pull did not spring back: %v", v.pullY.Current)
	}
}

func TestTextViewHorizontal(t *testing.T) {
	long := "this line is far too long to fit on a screen that is 128 pixels wide"
	v := newTestView(long, false)
	mx, _ := v.maxScroll()
	tap(v, KeyRight)
	if x, _ := v.Scroll(); x != viewStepX {
		t.Fatalf("right scrolled to %v, want %v", x, viewStepX)
	}
	for range 50 {
		tap(v, KeyRight)
	}
	if x, _ := v.Scroll(); x != mx {
		t.Fatalf("scrolled to %v, want %v", x, mx)
	}

	// Wrapping leaves nothing to the side.
	tap(v, KeyEnter)
	if !v.Wrap() {
		t.Fatal("enter did not turn wrapping on")
	}
	if x, _ := v.Scroll(); x != 0 {
		t.Fatalf("wrapped text is still scrolled sideways by %v", x)
	}
}

func TestTextViewWrapKeepsTopLine(t *testing.T) {
	var s string
	for i := range 30 {
		s += "paragraph " + string(rune('A'+i%26)) + " with some words in it that wrap\n"
	}
	v := newTestView(s, false)
	for range 10 {
		tap(v, KeyDown)
	}
	top := v.Line(10)

	v.SetWrap(true)
	_, y := v.Scroll()
	i := int(y / float32(v.LineHeight()))
	if got := v.Line(i); len(got) == 0 || got[:11] != top[:11] {
		t.Fatalf("top line is %q after wrapping, want the start of %q", got, top)
	}
	v.SetWrap(false)
	if _, y := v.Scroll(); y != 10*float32(v.LineHeight()) {
		t.Fatalf("unwrapping scrolled to %v, want line 10", y)
	}
}

func TestTextViewInViewer(t *testing.T) {
	c := NewCursor()
	vw := NewViewer(128)
	vw.Cursor = c
	home := NewList(NewText("a", nil), NewText("b", nil))
	home.Cursor = c
	home.SetTarget(RectF{W: 128, H: 64})
	vw.Push(home)
	run(vw, 1)

	tv := NewTextView("hello\nworld", nil, true)
	tv.Cursor = c
	tv.SetTarget(RectF{W: 128, H: 64})
	vw.Push(tv)
	if c.Target() != nil {
		t.Fatal("the text view did not hide the cursor")
	}
	run(vw, 1)

	// Left scrolls the view rather than going back; back goes back.
	tap(vw, KeyLeft)
	if vw.Top() != tv {
		t.Fatal("left went back from the text view")
	}
	tap(vw, KeyBack)
	run(vw, 1)
	if vw.Top() != home || c.Target() == nil {
		t.Fatal("back did not return to the list with the cursor")
	}
}

func TestTextViewDoesNotAllocate(t *testing.T) {
	v := newTestView("the quick brown fox\njumps over the lazy dog again and again and again", true)
	s := newScreen(128, 64)
	cv := NewCanvas(s)
	n := testing.AllocsPerRun(50, func() {
		v.Update(1.0 / 120)
		v.Draw(cv)
	})
	if n != 0 {
		t.Fatalf("%v allocations per frame", n)
	}
}

func TestRenderTextView(t *testing.T) {
	s := "osu!mc reader\nWrapped text breaks at spaces to fit the width of the screen.\n\tTabs, and a verylongwordthatcannotfitatall."
	for _, wrap := range []bool{true, false} {
		v := newTestView(s, wrap)
		tap(v, KeyDown)
		run(v, 1)
		t.Logf("wrap=%v\n%s", wrap, frame(v))
	}
}
