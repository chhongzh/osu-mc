package ui

// Scrollbar geometry, in pixels.
const (
	barReserve    = 6 // free space on the right: the wide thumb plus the cursor's padding
	barWideW      = 3 // thumb width while scrolling
	barThinW      = 1 // thumb width at rest
	barMinH       = 6
	barIdleTime   = 0.8 // seconds after the last scroll before the thumb thins
	barTrackPitch = 3   // one track dot every this many rows
)

// Stagger returns an Offset function that indents each item by base plus
// step for every position it is away from the selection, like the curved
// song carousel in osu!.
func Stagger(base, step float32) func(rel int) float32 {
	return func(rel int) float32 {
		return base + step*float32(absInt(rel))
	}
}

// List is a vertical list of items with a selection, smooth scrolling and a
// scrollbar. It is what a page is made of: lines of text, inputs, rows of
// chips and other lists, see ListItem.
//
// Every item is laid out from the list's target rectangle on every frame and
// springs towards its own slot, so moving or resizing the list, changing the
// selection, scrolling and adding or removing items all animate the same
// way. The springs keep their momentum: holding a key builds up speed, and
// letting go coasts the list past the selection and lets it settle back.
//
// A list is a Container: the selected item gets the keys first, and what it
// does not use comes back to the list. Up and down move the selection over
// the items that are Selectable, enter calls OnSelect, left and right call
// OnLeft and OnRight.
//
// Lists register animated values, create them with NewList and do not copy
// them. A list can be a page of a Viewer.
type List struct {
	AnimatedRect
	seq

	// ItemHeight overrides the height of every item; 0 means the height
	// each item asks for.
	ItemHeight float32

	// Offset gives the horizontal indent of an item from its position
	// relative to the selection (rel = index - selected, so 0 is the
	// selected item, -1 the one above). Nil means no indent. See Stagger.
	Offset func(rel int) float32

	// Marquee scrolls the selected item when it does not fit.
	Marquee bool

	// OnLeft and OnRight are called when left and right are released, once
	// per press even if the key repeated. Without them the keys are left to
	// the container: in a Viewer, left goes back.
	OnLeft, OnRight func()

	manual ListItem // the selected item scrolled by hand, its marquee paused

	bar     AnimatedRect
	barIdle float32
}

// NewList returns a centred list of items, with the first one that can be
// selected selected, that follows DefaultCursor.
func NewList(items ...ListItem) *List {
	l := &List{Marquee: true, barIdle: barIdleTime}
	l.init(true, items)
	l.Gap = 2
	l.Stagger = 0.2
	l.Cascade = 10
	l.InitRect(RectF{}, SpeedLayout)
	l.bar.InitRect(RectF{}, SpringBar)
	l.bar.SetSpring(SpringBar, BounceBar)
	return l
}

// scroller is an item that can be scrolled by hand, see ScrollSelected.
type scroller interface {
	SetMarquee(on bool)
	ScrollBy(delta float32)
}

// ScrollSelected scrolls the selected item horizontally by delta pixels and
// pauses its marquee until the selection changes.
func (l *List) ScrollSelected(delta float32) {
	it := l.SelectedItem()
	s, ok := it.(scroller)
	if !ok {
		return
	}
	l.manual = it
	s.SetMarquee(false)
	s.ScrollBy(delta)
}

// HandleEvent moves the selection with up and down, calls OnLeft and OnRight
// on left and right, and OnSelect on enter.
//
// Pressing a key only leans the cursor towards where it will go, or
// squeezes it for enter; the release does the rest. A held up or down moves
// on every repeat. Held against an end, the list is pulled that way until
// the key is let go.
func (l *List) HandleEvent(e Event) bool {
	switch e.Key {
	case KeyUp:
		return l.move(e, -1)
	case KeyDown:
		return l.move(e, 1)
	case KeyLeft, KeyRight:
		f, dir := l.OnLeft, float32(-1)
		if e.Key == KeyRight {
			f, dir = l.OnRight, 1
		}
		if f == nil {
			return false
		}
		// These are page actions, not steps: once per press, on
		// release, however long the key was held.
		step(l.Cursor, e, dir, 0)
		if e.Phase == Release {
			f()
		}
		return true
	case KeyEnter:
		return l.enter(e)
	}
	return false
}

// OnEnter is called by a Viewer when the list starts flying in. The items
// start further out the further they are from the selection, so they trail
// in behind it.
func (l *List) OnEnter(resume bool) {
	if !l.laidOut {
		l.Snap()
	}
	l.trail(enterSide(resume))
	l.Focus()
}

// OnLeave is called by a Viewer when the list starts flying out. The items
// are thrown against the direction of travel, so they lag behind.
func (l *List) OnLeave(exit bool) {
	l.lag(leaveKick(exit))
}

// Size is the list's own target size: a list inside another one is as
// high as it is made.
func (l *List) Size() (w, h float32) {
	t := l.Target()
	return t.W, t.H
}

// Selectable reports whether the list has an item that can be selected.
func (l *List) Selectable() bool { return l.selectable() }

// Release unregisters every animated value of the list and its items. A
// Viewer calls it once the list has flown out.
func (l *List) Release() {
	l.AnimatedRect.Release()
	l.bar.Release()
	l.release()
}

// Snap lays the list out and finishes all running animations.
func (l *List) Snap() {
	l.AnimatedRect.Snap()
	l.Layout()
	l.snap()
	l.bar.Snap()
}

func (l *List) itemHeight(it ListItem) float32 {
	if l.ItemHeight > 0 {
		return l.ItemHeight
	}
	_, h := it.Size()
	return h
}

// Layout sets the targets of the items and the scrollbar. Update calls it
// every frame; call it directly after changing the list outside of Update to
// get the new targets immediately.
func (l *List) Layout() {
	l.laidOut = true
	l.settle()
	v := l.Target()
	sel := max(l.selected, 0)

	// Content height and the slot of the selection.
	var y, selTop, selH float32
	for i, it := range l.items {
		h := l.itemHeight(it)
		if i == sel {
			selTop, selH = y, h
		}
		y += h + l.Gap
	}
	l.content = max(0, y-l.Gap)
	maxScroll := max(0, l.content-v.H)

	s := scrollTo(l.scroll, selTop, selH, l.content, v.H, l.Centered, l.ScrollMargin)
	if s != l.scroll {
		l.barIdle = 0
	}
	l.scroll = s

	reserve := float32(0)
	if maxScroll > 0 {
		reserve = barReserve
	}

	live := Axis(0)
	if l.Centered {
		live = AxisY
	}
	y = 0
	for i, it := range l.items {
		h := l.itemHeight(it)
		rel := i - sel
		var dx float32
		if l.Offset != nil {
			dx = l.Offset(rel)
		}
		f := it.Frame()
		f.SetTarget(RectF{v.X + dx, v.Y + y - s, v.W - reserve - dx, h})
		if lf, ok := it.(liveFocuser); ok {
			lf.setLiveFocus(live)
		}
		l.spring(f, rel)
		y += h + l.Gap
	}
	l.placeNew()

	if maxScroll == 0 {
		l.bar.SetTarget(RectF{v.X + v.W, v.Y, 0, v.H})
		return
	}
	thumbH := max(v.H*v.H/l.content, barMinH)
	w := float32(barThinW)
	if l.barIdle < barIdleTime {
		w = barWideW
	}
	l.bar.SetTarget(RectF{
		X: v.X + v.W - w,
		Y: v.Y + s/maxScroll*(v.H-thumbH),
		W: w,
		H: thumbH,
	})
}

func (l *List) Update(dt float32) {
	l.barIdle += dt
	it := l.SelectedItem()
	if it != l.manual {
		l.manual = nil
	}
	if m, ok := it.(marqueer); ok {
		m.SetMarquee(l.Marquee && l.manual == nil)
	}
	l.Layout()
	l.update(dt)
}

func (l *List) Draw(c Canvas) {
	r := l.Rect()
	c = c.Within(r)
	if c.Empty() {
		return
	}

	// The items are drawn pulled, the scrollbar is not.
	l.draw(c)

	if l.content > float32(r.H) {
		x := r.X + r.W - 1
		for y := r.Y; y < r.Y+r.H; y += barTrackPitch {
			c.Pixel(x, y)
		}
	}
	if b := l.bar.Rect(); !b.Empty() {
		c.FillRect(b.X, b.Y, b.W, b.H)
	}
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
