package ui

import "osu-mc/internal/animation"

// Animated is the eased value behind every coordinate in this package. It is
// stepped by animation.Default, never by the element that owns it.
type Animated = animation.Value

// Default easing speeds, in 1/s. See package animation for what they mean.
const (
	SpeedLayout = 14 // element rectangles
	SpeedScroll = 16 // text scroll offsets
)

// Default springs: natural frequency in rad/s and damping ratio. A critically
// damped spring at ω takes about as long to arrive as easing at ω/1.6, but it
// keeps its momentum when the target moves again.
const (
	SpringItem   = 22   // list items
	BounceScroll = 0.75 // vertical item motion, a little overshoot
	SpringBar    = 20   // scrollbar thumb
	BounceBar    = 0.8
	SpringPage   = 16 // viewer pages flying in and out
	BouncePage   = 0.85
	SpringPress  = 11 // cursor coming back from a press
	BouncePress  = 0.4
)
