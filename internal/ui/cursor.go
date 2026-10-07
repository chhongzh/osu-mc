package ui

import "osu-mc/internal/animation"

// Focusable is anything the cursor can sit on.
type Focusable interface {
	// FocusRect returns the area to cover, normally in target (not
	// current) coordinates, so the cursor heads for where the element is
	// going. See Text.LiveFocus for the exception.
	FocusRect() RectF
}

// Cursor is a selection highlight that glides between elements, in the
// spirit of the iPadOS pointer. It takes its size from whatever it is focused
// on and is drawn by inverting the pixels under it, so it works on top of
// anything.
//
// The four edges are animated separately rather than x, y, w, h: the edge on
// the side the cursor is moving towards uses the Lead speed and the opposite
// edge the slower Trail speed. The cursor therefore stretches towards its
// destination and contracts as it arrives.
//
// Pressing squeezes the cursor by PressDepth; on release it springs back
// with a little wobble.
//
// There is one cursor for the whole UI, and it lives in screen space: when
// the focus moves to another page it does not vanish with the old page and
// come in with the new one, it leaps across on its own, like a hero
// transition. Leap adds the jump: the cursor lifts, grows a little and lands
// with a squash.
type Cursor struct {
	left, top, right, bottom animation.Value

	Lead, Trail float32 // edge speeds, 1/s
	PadX, PadY  float32 // margin around the focus rectangle
	Radius      int16   // corner radius
	PressDepth  float32 // how far each side moves in while pressed
	LeapHeight  float32 // how high Leap lifts, roughly
	LeapGrow    float32 // how much each side grows at LeapHeight
	Hidden      bool

	target Focusable

	press   animation.Value // current squeeze, a spring
	pressed bool

	// Displacement springs on top of the focus position: lift for Leap
	// (up is positive), shiftX and shiftY for Nudge and Lean.
	lift   animation.Value
	shiftX animation.Value
	shiftY animation.Value
}

// The leap is slow and bouncy, so the landing squashes. Leaning is quicker.
const (
	springLeap = 10
	bounceLeap = 0.45
	springLean = 18
	bounceLean = 0.7
)

// pressInSpeed is how fast the cursor squeezes: quick and without bounce, the
// wobble is saved for the release.
const pressInSpeed = 45

// DefaultCursor is the cursor shared by the whole UI.
var DefaultCursor = NewCursor()

// NewCursor returns an unfocused cursor.
func NewCursor() *Cursor {
	c := &Cursor{
		Lead: 28, Trail: 12,
		PadX: 3, Radius: 3,
		PressDepth: 3,
		LeapHeight: 3, LeapGrow: 2,
	}
	c.left.Init(0, c.Lead)
	c.top.Init(0, c.Lead)
	c.right.Init(0, c.Lead)
	c.bottom.Init(0, c.Lead)
	c.press.Init(0, SpringPress)
	c.press.Spring(SpringPress, BouncePress)
	c.lift.Init(0, springLeap)
	c.lift.Spring(springLeap, bounceLeap)
	c.shiftX.Init(0, springLean)
	c.shiftX.Spring(springLean, bounceLean)
	c.shiftY.Init(0, springLean)
	c.shiftY.Spring(springLean, bounceLean)
	return c
}

// Focus moves the cursor onto f. Focusing nil hides it.
//
// Coming from nothing, the cursor grows out of the centre of f instead of
// flying in from the corner of the screen.
func (c *Cursor) Focus(f Focusable) {
	if c.target == nil && f != nil {
		r := c.goal(f)
		cx, cy := (r.X*2+r.W)/2, (r.Y*2+r.H)/2
		c.left.Jump(cx)
		c.right.Jump(cx)
		c.top.Jump(cy)
		c.bottom.Jump(cy)
	}
	c.target = f
}

// Target returns what the cursor is focused on.
func (c *Cursor) Target() Focusable {
	return c.target
}

// SetPressed squeezes the cursor while down is true and lets it spring back
// once it is false. Call it every frame with the state of the button.
func (c *Cursor) SetPressed(down bool) {
	if down == c.pressed {
		return
	}
	c.pressed = down
	if down {
		c.press.Spring(pressInSpeed, 1)
		c.press.Target = c.PressDepth
	} else {
		c.press.Spring(SpringPress, BouncePress)
		c.press.Target = 0
	}
}

// Pressed reports whether the cursor is held down.
func (c *Cursor) Pressed() bool {
	return c.pressed
}

// Bump plays a whole press and release at once, squeezing the cursor by
// about px on each side. Use it for clicks that have no held state.
func (c *Cursor) Bump(px float32) {
	if !c.pressed {
		c.press.Spring(SpringPress, BouncePress)
	}
	c.press.Kick(px)
}

// Leap makes the cursor jump: it lifts off, grows as if coming closer, and
// lands with a little squash. A Viewer calls it when the focus moves to
// another page, so the cursor hops across while the pages swap under it.
func (c *Cursor) Leap() {
	c.lift.Kick(c.LeapHeight)
}

// Nudge throws the cursor about px sideways and lets it spring back.
func (c *Cursor) Nudge(px float32) {
	c.shiftX.Kick(px)
}

// Lean moves the cursor by dx, dy and holds it there: anticipation while a
// key is down, before the release commits the move. Lean(0, 0) lets go.
func (c *Cursor) Lean(dx, dy float32) {
	c.shiftX.Target, c.shiftY.Target = dx, dy
}

// Snap puts the cursor on its target immediately.
func (c *Cursor) Snap() {
	c.Update(0)
	c.left.Snap()
	c.top.Snap()
	c.right.Snap()
	c.bottom.Snap()
	c.press.Snap()
	c.lift.Snap()
	c.shiftX.Snap()
	c.shiftY.Snap()
}

// Rect returns the current position of the cursor in pixels, press, leap and
// nudge included.
func (c *Cursor) Rect() Rect {
	lift := c.lift.Current
	p := c.press.Current // squeeze on each side
	if c.LeapHeight != 0 {
		p -= lift * c.LeapGrow / c.LeapHeight
	}
	dx, dy := c.shiftX.Current, c.shiftY.Current-lift
	x0 := animation.Round(c.left.Current + p + dx)
	y0 := animation.Round(c.top.Current + p/2 + dy)
	x1 := animation.Round(c.right.Current - p + dx)
	y1 := animation.Round(c.bottom.Current - p/2 + dy)
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

func (c *Cursor) Update(dt float32) {
	if c.target == nil {
		return
	}
	r := c.goal(c.target)
	c.left.Target, c.right.Target = r.X, r.X+r.W
	c.top.Target, c.bottom.Target = r.Y, r.Y+r.H

	lead(&c.left, &c.right, c.Lead, c.Trail)
	lead(&c.top, &c.bottom, c.Lead, c.Trail)
}

func (c *Cursor) Draw(cv Canvas) {
	if c.Hidden || c.target == nil {
		return
	}
	r := c.Rect()
	cv.InvertRoundRect(r.X, r.Y, r.W, r.H, c.Radius)
}

func (c *Cursor) goal(f Focusable) RectF {
	r := f.FocusRect()
	return RectF{r.X - c.PadX, r.Y - c.PadY, r.W + 2*c.PadX, r.H + 2*c.PadY}
}

// lead gives the edge on the side of travel the fast speed and the other edge
// the slow one. When the pair is not travelling (only resizing, or settled)
// both are fast.
func lead(lo, hi *animation.Value, fast, slow float32) {
	d := (lo.Target + hi.Target) - (lo.Current + hi.Current)
	switch {
	case d > 1:
		lo.Speed, hi.Speed = slow, fast
	case d < -1:
		lo.Speed, hi.Speed = fast, slow
	default:
		lo.Speed, hi.Speed = fast, fast
	}
}
