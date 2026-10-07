package ui

// Row is a horizontal list of items with a selection that scrolls sideways:
// tabs, chips, a row of keys. It is a ListItem itself, a line of a List.
//
// It moves like List turned on its side: every item springs towards its own
// slot, items further from the selection lag behind, and the selection is
// kept in the middle except near the ends. Left and right move the
// selection, enter calls OnSelect.
//
// Up and down are not the row's, so in a List they move the selection to
// the next line. Neither is left at the start when Pass has EdgeStart, the
// default: the key goes on to the container, and on a page that goes back.
//
// Rows register animated values, create them with NewRow and do not copy
// them.
type Row struct {
	AnimatedRect
	seq

	// ItemWidth sets the width of every item; 0 sizes each item to the
	// width it asks for plus Pad on both sides.
	ItemWidth float32
	Pad       float32
}

// NewRow returns a centred row of items with the first one that can be
// selected selected, that follows DefaultCursor.
func NewRow(items ...ListItem) *Row {
	r := &Row{Pad: 3}
	r.init(false, items)
	r.Gap = 2
	r.Stagger = 0.15
	r.Cascade = 6
	r.Pass = EdgeStart
	r.InitRect(RectF{H: RowHeight}, SpeedLayout)
	return r
}

// RowHeight is the height a new row starts with, one line of DefaultFont
// and a pixel.
const RowHeight = 16

// FocusNear selects the item closest to the middle of near, where the
// focus is coming from, and puts the cursor on it. Moving up and down
// through a List of rows thus keeps going straight.
func (r *Row) FocusNear(near RectF) {
	r.settle()
	if r.selected < 0 {
		return
	}
	r.Layout()
	cx := near.X + near.W/2
	best, bestD := r.selected, float32(-1)
	for i, it := range r.items {
		if !it.Selectable() {
			continue
		}
		t := it.Frame().Target()
		if d := abs(t.X + t.W/2 - cx); bestD < 0 || d < bestD {
			best, bestD = i, d
		}
	}
	if best != r.selected {
		deselect(r.items[r.selected])
		r.selected = best
	}
	r.Focus()
}

// HandleEvent moves the selection with left and right and calls OnSelect on
// enter, with the same press and release feedback as List.
func (r *Row) HandleEvent(e Event) bool {
	switch e.Key {
	case KeyLeft:
		return r.move(e, -1)
	case KeyRight:
		return r.move(e, 1)
	case KeyEnter:
		return r.enter(e)
	}
	return false
}

// OnEnter is called by a Viewer when the row starts flying in on its own.
func (r *Row) OnEnter(resume bool) {
	if !r.laidOut {
		r.Snap()
	}
	r.trail(enterSide(resume))
	r.Focus()
}

// OnLeave is called by a Viewer when the row starts flying out on its own.
func (r *Row) OnLeave(exit bool) {
	r.lag(leaveKick(exit))
}

// Size is the row's own target size; in a List that is one row high.
func (r *Row) Size() (w, h float32) {
	t := r.Target()
	return t.W, t.H
}

// Selectable reports whether the row has an item that can be selected.
func (r *Row) Selectable() bool { return r.selectable() }

// Release unregisters every animated value of the row and its items.
func (r *Row) Release() {
	r.AnimatedRect.Release()
	r.release()
}

// Snap lays the row out and finishes all running animations.
func (r *Row) Snap() {
	r.AnimatedRect.Snap()
	r.Layout()
	r.snap()
}

func (r *Row) itemWidth(it ListItem) float32 {
	if r.ItemWidth > 0 {
		return r.ItemWidth
	}
	w, _ := it.Size()
	return w + 2*r.Pad
}

// Layout sets the targets of the items. Update calls it every frame.
func (r *Row) Layout() {
	r.laidOut = true
	r.settle()
	v := r.Target()
	sel := max(r.selected, 0)

	var x, selX, selW float32
	for i, it := range r.items {
		w := r.itemWidth(it)
		if i == sel {
			selX, selW = x, w
		}
		x += w + r.Gap
	}
	r.content = max(0, x-r.Gap)
	r.scroll = scrollTo(r.scroll, selX, selW, r.content, v.W, r.Centered, r.ScrollMargin)

	live := Axis(0)
	if r.Centered {
		live = AxisX
	}
	x = 0
	for i, it := range r.items {
		w := r.itemWidth(it)
		f := it.Frame()
		f.SetTarget(RectF{v.X + x - r.scroll, v.Y, w, v.H})
		// A chip is a word in a box: the text sits in its middle.
		if t, ok := it.(*Text); ok {
			t.Center = true
		}
		if lf, ok := it.(liveFocuser); ok {
			lf.setLiveFocus(live)
		}
		r.spring(f, i-sel)
		x += w + r.Gap
	}
	r.placeNew()
}

func (r *Row) Update(dt float32) {
	r.Layout()
	r.update(dt)
}

// Draw clips the items to the row horizontally only, so a row that springs
// up and down in a List does not cut its items off.
func (r *Row) Draw(c Canvas) {
	rr := r.Rect()
	cl := c.Clip()
	c = c.Within(Rect{rr.X, cl.Y, rr.W, cl.H})
	if c.Empty() {
		return
	}
	r.draw(c)
}
