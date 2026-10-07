package ui

import (
	"image/color"
	"strings"
	"testing"

	"osu-mc/internal/animation"
)

// screen is a 1 bit frame buffer standing in for the display.
type screen struct {
	w, h int16
	px   []bool
}

func newScreen(w, h int16) *screen {
	return &screen{w: w, h: h, px: make([]bool, int(w)*int(h))}
}

func (s *screen) Size() (int16, int16) { return s.w, s.h }

func (s *screen) SetPixel(x, y int16, c color.RGBA) {
	if x < 0 || y < 0 || x >= s.w || y >= s.h {
		panic("SetPixel outside the screen")
	}
	s.px[int(y)*int(s.w)+int(x)] = c.R|c.G|c.B != 0
}

func (s *screen) InvertRect(x, y, w, h int16) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			s.px[int(j)*int(s.w)+int(i)] = !s.px[int(j)*int(s.w)+int(i)]
		}
	}
}

func (s *screen) at(x, y int16) bool { return s.px[int(y)*int(s.w)+int(x)] }

func (s *screen) String() string {
	var b strings.Builder
	for y := int16(0); y < s.h; y++ {
		for x := int16(0); x < s.w; x++ {
			if s.at(x, y) {
				b.WriteByte('#')
			} else {
				b.WriteByte('.')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// columns returns the range of columns with any pixel on, or -1, -1.
func (s *screen) columns() (int16, int16) {
	lo, hi := int16(-1), int16(-1)
	for x := int16(0); x < s.w; x++ {
		for y := int16(0); y < s.h; y++ {
			if s.at(x, y) {
				if lo < 0 {
					lo = x
				}
				hi = x
				break
			}
		}
	}
	return lo, hi
}

// tap presses and releases k on the element.
func tap(e Element, k Key) {
	r := Router{Root: e}
	r.Dispatch(Event{Key: k, Phase: Press})
	r.Dispatch(Event{Key: k, Phase: Release})
}

// run steps the element and the default manager for secs of frames.
func run(e Element, secs float32) {
	const dt = 1.0 / 120
	for t := float32(0); t < secs; t += dt {
		e.Update(dt)
		animation.Default.Update(dt)
	}
}

func frame(e Element) *screen {
	s := newScreen(128, 64)
	e.Draw(NewCanvas(s))
	return s
}

func TestTextClipsToWidth(t *testing.T) {
	txt := NewText("a long line that certainly does not fit", nil)
	txt.SetTarget(RectF{10, 5, 40, 16})
	txt.Snap()

	s := frame(txt)
	lo, hi := s.columns()
	if lo < 10 || hi >= 50 || lo < 0 {
		t.Fatalf("drew columns %d..%d, want within 10..49", lo, hi)
	}
}

func TestTextScrollClamps(t *testing.T) {
	txt := NewText("0123456789", nil)
	txt.SetTarget(RectF{0, 0, 30, 16})
	txt.Snap()

	txt.ScrollBy(-50)
	if txt.scroll.Target != 0 {
		t.Fatalf("scrolled to %v, want 0", txt.scroll.Target)
	}
	txt.ScrollBy(1000)
	if txt.scroll.Target != txt.Overflow() || txt.Overflow() <= 0 {
		t.Fatalf("scrolled to %v, want %v", txt.scroll.Target, txt.Overflow())
	}

	// Widening the text pulls the offset back into range.
	txt.W.Target = 1000
	txt.Update(0)
	if txt.scroll.Target != 0 {
		t.Fatalf("scroll %v after widening, want 0", txt.scroll.Target)
	}
}

func TestMarqueeCycles(t *testing.T) {
	txt := NewText("a long line that certainly does not fit", nil)
	txt.SetTarget(RectF{0, 0, 40, 16})
	txt.Snap()
	txt.SetMarquee(true)

	var reachedEnd, cameBack bool
	for i := 0; i < 120*20; i++ {
		txt.Update(1.0 / 120)
		animation.Default.Update(1.0 / 120)
		if txt.Scroll() > txt.Overflow()-0.5 {
			reachedEnd = true
		}
		if reachedEnd && txt.Scroll() < 0.5 {
			cameBack = true
			break
		}
	}
	if !reachedEnd || !cameBack {
		t.Fatalf("marquee end=%v back=%v", reachedEnd, cameBack)
	}
}

func newTestList(n int) *List {
	l := NewList()
	l.Cursor = NewCursor()
	for i := 0; i < n; i++ {
		l.Add(NewText(strings.Repeat("item ", i+1), nil))
	}
	l.ItemHeight = 16
	l.Offset = Stagger(4, 5)
	l.SetTarget(RectF{W: 128, H: 64})
	l.Snap()
	l.Focus()
	l.Cursor.Snap()
	return l
}

func TestListKeepsSelectionVisible(t *testing.T) {
	for _, centered := range []bool{true, false} {
		l := newTestList(10)
		l.Centered = centered
		root := Group{l, l.Cursor}
		for i := 0; i < 10; i++ {
			l.Move(1)
			run(root, 1)
			r := l.SelectedItem().Frame().Rect()
			if r.Y < 0 || r.Y+r.H > 64 {
				t.Fatalf("centered=%v: selection %d at y %d..%d", centered, l.Selected(), r.Y, r.Y+r.H)
			}
			c := l.Cursor.Rect()
			if c.Y != r.Y || c.H != r.H {
				t.Fatalf("centered=%v: cursor at y %d h %d, item at y %d h %d", centered, c.Y, c.H, r.Y, r.H)
			}
		}
		if l.Selected() != 9 {
			t.Fatalf("selected %d, want 9 (clamped)", l.Selected())
		}
		if b := l.bar.Rect(); b.Y+b.H != 64 {
			t.Fatalf("centered=%v: scrollbar thumb %+v not at the bottom", centered, b)
		}
	}
}

func TestListCentersSelection(t *testing.T) {
	l := newTestList(10)
	check := func(i int, want func(r Rect) bool, what string) {
		t.Helper()
		l.Select(i)
		run(Group{l, l.Cursor}, 1)
		if r := l.SelectedItem().Frame().Rect(); !want(r) {
			t.Errorf("item %d at y %d..%d, want %s", i, r.Y, r.Y+r.H, what)
		}
	}
	// The ends stop at the edges instead of being centred.
	check(0, func(r Rect) bool { return r.Y == 0 }, "at the top")
	check(1, func(r Rect) bool { return r.Y == 18 }, "right below the first")
	check(4, func(r Rect) bool { return r.Y+r.H/2 == 32 }, "centred")
	check(9, func(r Rect) bool { return r.Y+r.H == 64 }, "at the bottom")
	if b := l.bar.Rect(); b.Y+b.H != 64 {
		t.Errorf("scrollbar thumb %+v not at the bottom with the last item selected", b)
	}
}

// Scrolling fast and stopping coasts past the selection and settles back.
func TestListInertia(t *testing.T) {
	l := newTestList(20)
	root := Group{l, l.Cursor}
	for i := 0; i < 12; i++ {
		l.Move(1)
		run(root, 0.035)
	}
	it := l.SelectedItem().Frame()
	var over float32
	for i := 0; i < 120; i++ {
		run(root, 1.0/120)
		over = min(over, it.Y.Current-it.Y.Target)
	}
	if over > -1 {
		t.Fatalf("overshoot %v px, want the list to coast past", over)
	}
	if !it.Y.Settled() {
		t.Fatalf("still moving after a second: %v -> %v", it.Y.Current, it.Y.Target)
	}
}

// Held against an end, a list is pulled that way, cursor and all, and
// stays there until the key is let go; nothing is thrown, so nothing bounces.
func TestListEdgePulls(t *testing.T) {
	l := newTestList(4)
	c := l.Cursor
	r := Router{Root: l}
	rest, cur := l.Item(0).Frame().Target().Y, c.Rect().Y

	r.Dispatch(Event{Key: KeyUp, Phase: Press})
	run(l, 0.5)
	c.Update(0)
	if l.pull.Int() != pullDist || c.Rect().Y != cur+pullDist {
		t.Fatalf("held: list at %v, cursor at %v, want both %v", l.pull.Current, c.Rect().Y-cur, pullDist)
	}
	r.Dispatch(Event{Key: KeyUp, Phase: Repeat})
	run(l, 0.5)
	if l.Selected() != 0 || l.pull.Int() != pullDist || l.Item(0).Frame().Target().Y != rest {
		t.Fatalf("repeat moved the list: selected %d, pull %v", l.Selected(), l.pull.Current)
	}

	r.Dispatch(Event{Key: KeyUp, Phase: Release, Repeated: true})
	var past float32
	for i := 0; i < 120; i++ {
		run(l, 1.0/120)
		past = min(past, l.pull.Current)
	}
	if l.pull.Current != 0 || past < -0.5 {
		t.Fatalf("after release the list is at %v, swung %v past its place", l.pull.Current, past)
	}
}

// A row in a list pulls the same way, along its own axis, and the list
// along its own.
func TestRowInListEdgesPull(t *testing.T) {
	c := NewCursor()
	row := newTestRow(c, 3)
	row.Pass = 0
	col := newTestLines(c, row)
	col.OnEnter(false)
	r := Router{Root: col}

	r.Dispatch(Event{Key: KeyLeft, Phase: Press})
	r.Dispatch(Event{Key: KeyDown, Phase: Press})
	run(col, 0.5)
	if row.pull.Int() != -pullDist || col.pull.Int() != -pullDist {
		t.Fatalf("row pulled %v, column %v", row.pull.Current, col.pull.Current)
	}
	r.Dispatch(Event{Key: KeyLeft, Phase: Release})
	r.Dispatch(Event{Key: KeyDown, Phase: Release})
	run(col, 1)
	if row.pull.Current != 0 || col.pull.Current != 0 || row.Selected() != 0 {
		t.Fatal("did not come back")
	}
}

func TestListStagger(t *testing.T) {
	l := newTestList(4)
	l.Select(1)
	run(l, 2)
	want := []float32{9, 4, 9, 14}
	for i := range want {
		if x := l.Item(i).Frame().X.Current; x != want[i] {
			t.Errorf("item %d at x %v, want %v", i, x, want[i])
		}
	}
}

func TestCursorStretchesTowardsTarget(t *testing.T) {
	l := newTestList(4)
	before := l.Cursor.Rect()
	l.Move(1)
	Group{l, l.Cursor}.Update(0)
	animation.Default.Update(1.0 / 60)
	mid := l.Cursor.Rect()
	if mid.H <= before.H {
		t.Fatalf("cursor height %d while moving, want more than %d", mid.H, before.H)
	}
}

func TestCursorPressBouncesBack(t *testing.T) {
	l := newTestList(4)
	root := Group{l, l.Cursor}
	rest := l.Cursor.Rect()

	l.Cursor.SetPressed(true)
	run(root, 0.2)
	if r := l.Cursor.Rect(); r.W != rest.W-6 || r.H != rest.H-3 {
		t.Fatalf("pressed %+v, want 6 narrower and 3 lower than %+v", r, rest)
	}

	l.Cursor.SetPressed(false)
	var widest int16
	for i := 0; i < 240; i++ {
		run(root, 1.0/120)
		widest = max(widest, l.Cursor.Rect().W)
	}
	if widest <= rest.W {
		t.Fatalf("released cursor never grew past its rest width %d", rest.W)
	}
	if r := l.Cursor.Rect(); r != rest {
		t.Fatalf("cursor at %+v after release, want %+v", r, rest)
	}
}

func TestCanvasTranslate(t *testing.T) {
	s := newScreen(128, 64)
	c := NewCanvas(s).Translate(100, 0)
	if r := c.Clip(); r.X != -100 || r.W != 128 {
		t.Fatalf("local clip %+v", r)
	}
	c = c.Within(Rect{0, 0, 50, 10})
	c.FillRect(-10, 0, 100, 100)
	if lo, hi := s.columns(); lo != 100 || hi != 127 {
		t.Fatalf("filled columns %d..%d, want 100..127", lo, hi)
	}
}

func newPage(c *Cursor, n int) *List {
	l := NewList()
	l.Cursor = c
	for i := 0; i < n; i++ {
		l.Add(NewText(strings.Repeat("page ", i+1), nil))
	}
	l.ItemHeight = 16
	l.Offset = Stagger(4, 5)
	l.SetTarget(RectF{W: 128, H: 64})
	return l
}

func TestViewer(t *testing.T) {
	v := NewViewer(128)
	v.Cursor = NewCursor()
	a := newPage(v.Cursor, 5)
	v.Push(a)
	state := func(e Element, want PageState) {
		t.Helper()
		if got, ok := v.State(e); !ok || got != want {
			t.Fatalf("page is %v (in viewer %v), want %v", got, ok, want)
		}
	}
	state(a, PageEntering)
	run(v, 2)
	state(a, PageActive)
	if v.Cursor.Target() != Focusable(a.SelectedItem().(*Text)) {
		t.Fatal("cursor not on the first page")
	}

	before := animation.Default.Len()
	var b *List
	a.OnSelect = func(i int) {
		b = newPage(v.Cursor, 3)
		v.Push(b)
	}
	tap(v, KeyDown)
	tap(v, KeyEnter)
	state(a, PageSuspending)
	state(b, PageEntering)

	// Half way: both pages are on screen, and nothing is drawn outside it.
	run(v, 0.08)
	if lo, hi := frame(v).columns(); lo > 10 || hi < 118 {
		t.Fatalf("mid flight drew columns %d..%d, want both pages", lo, hi)
	}
	run(v, 2)
	state(a, PageSuspended)
	state(b, PageActive)
	if v.Cursor.Target() != Focusable(b.SelectedItem().(*Text)) {
		t.Fatal("cursor did not follow the new page")
	}

	tap(v, KeyDown)
	if b.Selected() != 1 || a.Selected() != 1 {
		t.Fatalf("keys went to the wrong page: a=%d b=%d", a.Selected(), b.Selected())
	}

	tap(v, KeyBack)
	state(a, PageEntering)
	state(b, PageExiting)
	run(v, 2)
	state(a, PageActive)
	if _, ok := v.State(b); ok {
		t.Fatal("popped page still in the viewer")
	}
	if n := animation.Default.Len(); n != before {
		t.Fatalf("%d animated values registered, want %d: popped page not released", n, before)
	}
	if v.Cursor.Target() != Focusable(a.Item(1).(*Text)) {
		t.Fatal("cursor did not come back to the first page")
	}
	if v.Pop() {
		t.Fatal("popped the root page")
	}
}

// The cursor is shared: switching pages must not make it jump by a page
// width, it flies across on its own and hops on the way.
func TestViewerHeroCursor(t *testing.T) {
	v := NewViewer(128)
	v.Cursor = NewCursor()
	a := newPage(v.Cursor, 5)
	v.Push(a)
	run(v, 2)
	rest := v.Cursor.Rect()

	for _, change := range []func(){
		func() { v.Push(newPage(v.Cursor, 3)) },
		func() { v.Back() },
	} {
		change()
		prev := v.Cursor.Rect()
		lifted := false
		for i := 0; i < 240; i++ {
			run(v, 1.0/120)
			r := v.Cursor.Rect()
			if d := absInt(int(r.X - prev.X)); d > 8 {
				t.Fatalf("cursor jumped %d px in a frame: %+v -> %+v", d, prev, r)
			}
			if r.Y < rest.Y {
				lifted = true
			}
			prev = r
		}
		if !lifted {
			t.Fatal("cursor did not leap")
		}
		if r := v.Cursor.Rect(); r != rest {
			t.Fatalf("cursor landed at %+v, want %+v", r, rest)
		}
	}
}

func TestViewerBackOnRoot(t *testing.T) {
	v := NewViewer(128)
	v.Cursor = NewCursor()
	a := newPage(v.Cursor, 3)
	v.Push(a)
	run(v, 2)

	tap(v, KeyLeft)
	var gave float32
	for i := 0; i < 120; i++ {
		run(v, 1.0/120)
		gave = max(gave, v.stack[0].x.Current)
	}
	if v.Len() != 1 || gave < 3 {
		t.Fatalf("%d pages, root gave %v px; want 1 and a shove", v.Len(), gave)
	}
	if st, _ := v.State(a); st != PageActive || v.stack[0].x.Current != 0 {
		t.Fatalf("root %v at x %v after the shove", st, v.stack[0].x.Current)
	}

	b := newPage(v.Cursor, 3)
	v.Push(b)
	run(v, 2)
	tap(v, KeyLeft)
	if st, _ := v.State(b); st != PageExiting {
		t.Fatalf("left on the second page left it %v, want exiting", st)
	}
}

func TestViewerPopToRoot(t *testing.T) {
	v := NewViewer(128)
	v.Cursor = NewCursor()
	root := newPage(v.Cursor, 3)
	v.Push(root)
	run(v, 1)
	before := animation.Default.Len()
	for i := 0; i < 3; i++ {
		v.Push(newPage(v.Cursor, 3))
		run(v, 1)
	}
	if !v.PopToRoot() || v.Len() != 1 || v.Top() != Element(root) {
		t.Fatalf("PopToRoot left %d pages", v.Len())
	}
	run(v, 2)
	if n := animation.Default.Len(); n != before {
		t.Fatalf("%d animated values registered, want %d", n, before)
	}
}

// TestRender prints a frame with go test -v, to eyeball the layout.
func TestRender(t *testing.T) {
	l := newTestList(6)
	l.Item(2).(*Text).SetText("Hige Driver - Miracle Sugite Yabai (feat. Ayane)")
	root := Group{l, l.Cursor}
	l.Move(2)
	run(root, 2)
	t.Log("\n" + frame(root).String())

	// A viewer in the middle of pushing a second page.
	v := NewViewer(128)
	v.Cursor = NewCursor()
	v.Push(newPage(v.Cursor, 6))
	run(v, 2)
	v.Push(newPage(v.Cursor, 4))
	run(v, 0.1)
	t.Log("\n" + frame(v).String())
}
