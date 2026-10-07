package ui

import "osu-mc/internal/animation"

// Edge is a set of ends of a List or a Row.
type Edge uint8

const (
	EdgeStart Edge = 1 << iota // before the first item: up, or left
	EdgeEnd                    // after the last item: down, or right
)

// leanDist is how far the cursor leans towards where a held key is about to
// take it.
const leanDist = 2

// step is the press and release half of moving a selection by one item
// towards dx, dy: the press leans the cursor that way, the release lets it
// go. It reports whether the move should happen now, see Event.Fires.
func step(c *Cursor, e Event, dx, dy float32) bool {
	if c != nil {
		switch e.Phase {
		case Press:
			c.Lean(dx*leanDist, dy*leanDist)
		case Release:
			c.Lean(0, 0)
		}
	}
	return e.Fires()
}

// pullDist is how far content is pulled past its end while a key pushes it
// there.
const pullDist = 6

// initPull sets up the offset of content pulled past its end. It springs
// like the cursor's lean, so the content and the cursor move as one.
func initPull(v *animation.Value) {
	v.Init(0, springLean)
	v.Spring(springLean, bounceLean)
}

// push is step for a key that may push the selection past its end, blocked
// telling whether it does. dx, dy is the direction of travel, used for the
// ordinary lean; pdx, pdy is the direction the content and cursor are
// pulled towards when blocked, which for the vertical axis is the opposite
// of dx, dy to match the sideways over-drag's feel on a gamepad's stick.
// Held against an end, the content (pull, drawn as an offset) and the
// cursor are pulled towards pdx, pdy by pullDist, like a page peeking
// while back is held; the release lets both spring back. Nothing is
// thrown, so nothing bounces. It reports whether the move should happen
// now.
func push(c *Cursor, pull *animation.Value, e Event, blocked bool, dx, dy, pdx, pdy float32) bool {
	if e.Phase == Release {
		pull.Target = 0
	} else if blocked {
		pull.Target = (pdx + pdy) * pullDist
		if c != nil {
			c.Lean(pdx*pullDist, pdy*pullDist)
		}
		return false
	}
	return step(c, e, dx, dy)
}

// click is the press and release half of a button: the press squeezes the
// cursor, the release lets it spring back. It reports whether the action
// should happen now, which is on release only.
func click(c *Cursor, e Event) bool {
	if c != nil {
		switch e.Phase {
		case Press:
			c.SetPressed(true)
		case Release:
			c.SetPressed(false)
		}
	}
	return e.Phase == Release
}

// scrollTo returns the scroll offset along one axis that shows the selection,
// which covers start..start+size of content, in a viewport of length view.
//
// Centred keeps the selection in the middle, except near the ends, where the
// content stops at its first or last item instead of leaving half the
// viewport empty. Otherwise the offset moves from s only when the selection
// comes closer than margin to an edge.
func scrollTo(s, start, size, content, view float32, centered bool, margin float32) float32 {
	if centered {
		s = start + size/2 - view/2
	} else {
		if start-margin < s {
			s = start - margin
		}
		if start+size+margin > s+view {
			s = start + size + margin - view
		}
	}
	return clampf(s, 0, max(0, content-view))
}

// stagger returns the spring frequency of an item rel places away from the
// selection: the further away, the slower, so a list moves as a wave.
func stagger(speed, factor float32, rel int) float32 {
	return speed / (1 + factor*float32(absInt(rel)))
}

// enterSide is the side a page comes in from: the right when pushed, the
// left when resumed.
func enterSide(resume bool) float32 {
	if resume {
		return -1
	}
	return 1
}

// leaveKick is the way content is thrown so it lags behind a page flying
// out: right when the page leaves to the left under a new one, left when it
// is popped off to the right.
func leaveKick(exit bool) float32 {
	if exit {
		return -1
	}
	return 1
}
