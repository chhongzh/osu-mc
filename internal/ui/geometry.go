package ui

import "osu-mc/internal/animation"

// Rect is a pixel rectangle.
type Rect struct {
	X, Y, W, H int16
}

// Empty reports whether the rectangle covers no pixels.
func (r Rect) Empty() bool {
	return r.W <= 0 || r.H <= 0
}

// Intersect returns the overlap of r and o, or an empty rectangle.
func (r Rect) Intersect(o Rect) Rect {
	x0, y0 := max(r.X, o.X), max(r.Y, o.Y)
	x1, y1 := min(r.X+r.W, o.X+o.W), min(r.Y+r.H, o.Y+o.H)
	if x1 < x0 {
		x1 = x0
	}
	if y1 < y0 {
		y1 = y0
	}
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

// RectF is a rectangle in sub-pixel layout space.
type RectF struct {
	X, Y, W, H float32
}

// AnimatedRect is a rectangle whose four components ease independently.
//
// Elements only ever write targets; the animation manager moves the current
// values, and drawing uses the current values.
type AnimatedRect struct {
	X, Y, W, H animation.Value
}

// InitRect places the rectangle at r without animation and registers its
// values with the default manager.
func (a *AnimatedRect) InitRect(r RectF, speed float32) {
	a.X.Init(r.X, speed)
	a.Y.Init(r.Y, speed)
	a.W.Init(r.W, speed)
	a.H.Init(r.H, speed)
}

// Frame returns the rectangle itself. Every element that embeds an
// AnimatedRect gets it, which is how containers place their children.
func (a *AnimatedRect) Frame() *AnimatedRect {
	return a
}

// SetTarget sets where the rectangle should end up.
func (a *AnimatedRect) SetTarget(r RectF) {
	a.X.Target, a.Y.Target, a.W.Target, a.H.Target = r.X, r.Y, r.W, r.H
}

// Target returns where the rectangle is heading. Layout should always be
// computed from targets, so that parents and children do not compound each
// other's easing.
func (a *AnimatedRect) Target() RectF {
	return RectF{a.X.Target, a.Y.Target, a.W.Target, a.H.Target}
}

// Current returns where the rectangle is right now.
func (a *AnimatedRect) Current() RectF {
	return RectF{a.X.Current, a.Y.Current, a.W.Current, a.H.Current}
}

// Rect returns the current position in whole pixels. The far edges are
// rounded rather than the size, so a moving rectangle does not wobble in
// width by a pixel as the two edges cross .5 at different times.
func (a *AnimatedRect) Rect() Rect {
	x0 := animation.Round(a.X.Current)
	y0 := animation.Round(a.Y.Current)
	x1 := animation.Round(a.X.Current + a.W.Current)
	y1 := animation.Round(a.Y.Current + a.H.Current)
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

// SetSpeed sets the easing speed of all four components.
func (a *AnimatedRect) SetSpeed(speed float32) {
	a.X.Speed, a.Y.Speed, a.W.Speed, a.H.Speed = speed, speed, speed, speed
}

// SetSpring switches all four components to spring motion, see
// animation.Value.Spring.
func (a *AnimatedRect) SetSpring(speed, damping float32) {
	a.X.Spring(speed, damping)
	a.Y.Spring(speed, damping)
	a.W.Spring(speed, damping)
	a.H.Spring(speed, damping)
}

// Settled reports whether all four components have stopped.
func (a *AnimatedRect) Settled() bool {
	return a.X.Settled() && a.Y.Settled() && a.W.Settled() && a.H.Settled()
}

// Release unregisters the four components from the animation manager.
func (a *AnimatedRect) Release() {
	a.X.Release()
	a.Y.Release()
	a.W.Release()
	a.H.Release()
}

// Snap finishes any running animation.
func (a *AnimatedRect) Snap() {
	a.X.Snap()
	a.Y.Snap()
	a.W.Snap()
	a.H.Snap()
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func clampf(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
