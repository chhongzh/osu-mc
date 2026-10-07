package ui

import "osu-mc/internal/animation"

// The box at the end of the line, in pixels, and how its mark moves, in
// rad/s.
const (
	boxSize     = 9    // the outline is square, this many pixels on a side
	boxInset    = 2    // space the outline keeps around a mark grown to 1
	springCheck = 26   // the mark popping in and out of the box
	bounceCheck = 0.45 // overshoot, so a tick lands with a snap
)

// Checkbox is a label with a box at the end of the line that is either
// ticked or not. Enter toggles it, and the mark pops out of the middle of the
// box and springs past the size it rests at, so a tick lands hard.
//
// It is a ListItem: it goes in a List as one of its lines, the way a settings
// page is a list of switches. The list places the checkbox's frame; the label
// sits inside it, Pad in from the left, and the box sits Pad in from the
// right with at least Gap between the two. A label too long for what is left
// is clipped.
//
// Only enter is the checkbox's. Left and right are left to the container, so
// that in a Viewer left still goes back.
//
// Checkboxes register animated values, create them with NewCheckbox and do
// not copy them.
type Checkbox struct {
	AnimatedRect

	// Pad is the space left of the label and right of the box.
	Pad float32

	// Gap is the least space kept between the label and the box.
	Gap float32

	// Cursor is squeezed while enter is held, see click.
	Cursor *Cursor

	// OnChange is called with the new state when enter is released, after
	// the box has been toggled. Setting the state in code does not call it.
	OnChange func(checked bool)

	label   *Text
	checked bool
	live    Axis

	// fill is how far the mark has grown into the box, 0 to 1. It springs
	// past 1, and the mark is capped at the outline rather than clamped, so
	// the overshoot is what is seen.
	fill animation.Value
}

// NewCheckbox returns an unticked checkbox, one row high, labelled s. It has
// no Pad, so in a List its label lines up with the texts around it.
func NewCheckbox(s string) *Checkbox {
	b := &Checkbox{Gap: 4, Cursor: DefaultCursor, label: NewText(s, nil)}
	b.InitRect(RectF{H: RowHeight}, SpeedLayout)
	b.fill.Init(0, springCheck)
	b.fill.Spring(springCheck, bounceCheck)
	b.place()
	return b
}

// Label returns the Text that draws the label, to change its font.
func (b *Checkbox) Label() *Text { return b.label }

// SetLabel replaces the label.
func (b *Checkbox) SetLabel(s string) { b.label.SetText(s) }

// Checked reports whether the box is ticked.
func (b *Checkbox) Checked() bool { return b.checked }

// SetChecked ticks or unticks the box, the mark growing or shrinking into
// place. It does not call OnChange.
func (b *Checkbox) SetChecked(on bool) {
	if on == b.checked {
		return
	}
	b.checked = on
	b.fill.Target = 0
	if on {
		b.fill.Target = 1
	}
}

// Toggle flips the box and calls OnChange with the new state.
func (b *Checkbox) Toggle() {
	b.SetChecked(!b.checked)
	if b.OnChange != nil {
		b.OnChange(b.checked)
	}
}

// Size is as wide as the label, the gap, the box and the padding, and as
// high as the checkbox is made.
func (b *Checkbox) Size() (w, h float32) {
	return 2*b.Pad + float32(b.label.ContentWidth()) + b.Gap + boxSize, b.H.Target
}

// Selectable reports true: a checkbox is there to be toggled.
func (b *Checkbox) Selectable() bool { return true }

func (b *Checkbox) setLiveFocus(a Axis) { b.live = a }

// FocusRect is the whole line, so the cursor covers the label and the box
// together: the box is part of the control, not something beside it.
func (b *Checkbox) FocusRect() RectF {
	r := b.Target()
	if b.live&AxisX != 0 {
		r.X = b.X.Current
	}
	if b.live&AxisY != 0 {
		r.Y = b.Y.Current
	}
	return r
}

// HandleEvent toggles the box when enter is released, squeezing the cursor
// while it is held.
func (b *Checkbox) HandleEvent(e Event) bool {
	if e.Key != KeyEnter {
		return false
	}
	if click(b.Cursor, e) {
		b.Toggle()
	}
	return true
}

// Release unregisters the checkbox's animated values.
func (b *Checkbox) Release() {
	b.AnimatedRect.Release()
	b.fill.Release()
	b.label.Release()
}

// Snap finishes the checkbox's animations, the mark included.
func (b *Checkbox) Snap() {
	b.AnimatedRect.Snap()
	b.fill.Snap()
	b.place()
}

// place puts the label inside the frame, in the space the box leaves. The
// label is carried along with the frame rather than springing after it, so
// the two never drift apart.
func (b *Checkbox) place() {
	t, c := b.Target(), b.Current()
	room := b.Gap + boxSize + 2*b.Pad
	b.label.SetTarget(RectF{t.X + b.Pad, t.Y, max(0, t.W-room), t.H})
	b.label.X.Current = c.X + b.Pad
	b.label.Y.Current = c.Y
	b.label.W.Current = max(0, c.W-room)
	b.label.H.Current = c.H
}

func (b *Checkbox) Update(dt float32) {
	b.place()
	b.label.Update(dt)
}

func (b *Checkbox) Draw(c Canvas) {
	b.label.Draw(c)

	r := b.Rect()
	x := r.X + r.W - animation.Round(b.Pad) - boxSize
	y := r.Y + (r.H-boxSize)/2
	c.FillRect(x, y, boxSize, 1)
	c.FillRect(x, y+boxSize-1, boxSize, 1)
	c.FillRect(x, y+1, 1, boxSize-2)
	c.FillRect(x+boxSize-1, y+1, 1, boxSize-2)

	// The mark grows out of the middle of the box. The spring overshoots, so
	// it is stopped where it would swallow the outline rather than at the
	// size it rests at.
	d := min(animation.Round(b.fill.Current*(boxSize-2*boxInset)), boxSize-2)
	if d <= 0 {
		return
	}
	c.FillRect(x+(boxSize-d)/2, y+(boxSize-d)/2, d, d)
}
