// Package animation provides eased values that are advanced once per frame by
// a shared Manager.
//
// Every value is exponential smoothing towards its target:
//
//	current += (target - current) * (1 - e^(-speed*dt))
//
// Unlike the naive current += diff*speed*dt this never overshoots and behaves
// the same at 30 and at 120 fps. Speed is in 1/s: after 1/speed seconds about
// 63% of the distance has been covered, after 3/speed about 95%.
//
// A value with a non-zero Damping is a damped spring instead. It carries a
// velocity, so a target that keeps moving builds up momentum and a value that
// was moving fast coasts past its target and swings back. That is what gives
// scrolling its inertia. Speed is then the natural frequency in rad/s and
// Damping the damping ratio: 1 is critically damped (no overshoot, but still
// with inertia), below 1 bounces, and the lower it is the more.
package animation

const (
	// settleEpsilon is how close a value has to get to its target before
	// it snaps. Positions are rounded to whole pixels, so anything well
	// below one pixel is invisible.
	settleEpsilon = 0.01

	// A spring also has to be nearly at rest, in px/s, before it snaps.
	springSettlePos = 0.05
	springSettleVel = 0.5

	// springStep is the largest integration step. Semi-implicit Euler is
	// stable while Speed*step < 2; 1/240 s keeps that for any sensible
	// speed and costs two steps per frame at 120 fps.
	springStep = 1.0 / 240

	e = 2.7182817
)

// Value is an eased float. Set Target and let the manager move Current.
//
// A Value that is registered with a manager must not be copied, the manager
// keeps a pointer to it.
type Value struct {
	Current float32
	Target  float32
	Speed   float32

	// Damping switches the value to spring motion when non-zero, see the
	// package comment. Velocity is only used by springs.
	Damping  float32
	Velocity float32

	// idx is the slot in the owning manager, or -1 when unregistered.
	idx int32
}

// New allocates a value at v and registers it with Default.
func New(v, speed float32) *Value {
	a := &Value{}
	a.Init(v, speed)
	return a
}

// Init sets the value to v with no pending motion and registers it with
// Default. It is meant for values embedded in other structs.
func (a *Value) Init(v, speed float32) {
	a.Current, a.Target, a.Speed = v, v, speed
	a.idx = -1
	Default.Register(a)
}

// NewSpring allocates a spring at v and registers it with Default.
func NewSpring(v, speed, damping float32) *Value {
	a := New(v, speed)
	a.Damping = damping
	return a
}

// Spring switches the value to spring motion with the given natural
// frequency and damping ratio. A damping of 0 switches back to plain easing.
func (a *Value) Spring(speed, damping float32) {
	a.Speed, a.Damping = speed, damping
	if damping == 0 {
		a.Velocity = 0
	}
}

// Release unregisters the value from Default. Call it when the owner is
// thrown away, otherwise the manager keeps stepping it forever.
func (a *Value) Release() {
	Default.Unregister(a)
}

// Kick throws a spring off its target: it gains enough velocity that, when
// critically damped and at rest, it swings about px away before coming back.
// Bouncier springs swing somewhat further. Eased values ignore it.
func (a *Value) Kick(px float32) {
	if a.Damping != 0 {
		a.Velocity += px * a.Speed * e
	}
}

// Set sets the target. The value eases towards it over the next frames.
func (a *Value) Set(target float32) {
	a.Target = target
}

// Add moves the target by delta.
func (a *Value) Add(delta float32) {
	a.Target += delta
}

// Jump moves both current and target to v, without animation.
func (a *Value) Jump(v float32) {
	a.Current, a.Target, a.Velocity = v, v, 0
}

// Snap finishes the animation immediately.
func (a *Value) Snap() {
	a.Current, a.Velocity = a.Target, 0
}

// Settled reports whether the value has reached its target and stopped.
func (a *Value) Settled() bool {
	return a.Current == a.Target && a.Velocity == 0
}

// Int returns the current value rounded to the nearest integer.
func (a *Value) Int() int16 {
	return Round(a.Current)
}

// step advances the value by dt seconds and reports whether it changed.
func (a *Value) step(dt float32) bool {
	if a.Damping != 0 {
		return a.stepSpring(dt)
	}
	d := a.Target - a.Current
	if d == 0 {
		return false
	}
	if d < settleEpsilon && d > -settleEpsilon {
		a.Current = a.Target
		return true
	}
	a.Current += d * (1 - expNeg(a.Speed*dt))
	return true
}

func (a *Value) stepSpring(dt float32) bool {
	d := a.Target - a.Current
	if d == 0 && a.Velocity == 0 {
		return false
	}
	if abs(d) < springSettlePos && abs(a.Velocity) < springSettleVel {
		a.Current, a.Velocity = a.Target, 0
		return true
	}

	k := a.Speed * a.Speed
	c := 2 * a.Damping * a.Speed
	for dt > 0 {
		h := min(dt, springStep)
		a.Velocity += (k*(a.Target-a.Current) - c*a.Velocity) * h
		a.Current += a.Velocity * h
		dt -= h
	}
	return true
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// Round rounds to the nearest integer, halves away from zero. A plain int16()
// conversion truncates towards zero, which makes anything moving across 0
// stall for an extra pixel.
func Round(v float32) int16 {
	if v < 0 {
		return int16(v - 0.5)
	}
	return int16(v + 0.5)
}

// expNeg approximates e^(-x) for x >= 0.
//
// The ESP32-S3 FPU is single precision only, math.Exp is float64 and would be
// done in software for every animated value on every frame. Instead the
// argument is halved until it is small, a (2,2) Padé approximant is applied,
// and the result is squared back up. The error is far below what shows up
// after rounding to pixels.
func expNeg(x float32) float32 {
	if x <= 0 {
		return 1
	}
	if x > 16 {
		return 0
	}
	n := 0
	for x > 0.5 {
		x *= 0.5
		n++
	}
	x2 := x * x / 12
	h := x / 2
	r := (1 - h + x2) / (1 + h + x2)
	for ; n > 0; n-- {
		r *= r
	}
	return r
}
