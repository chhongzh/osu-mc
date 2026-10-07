package ui

import "osu-mc/internal/animation"

// The bar under a chip that is on, in pixels, and how it moves, in rad/s.
const (
	chipMarkH  = 2    // the bar is this many pixels tall
	springChip = 24   // the bar growing out of the middle of the chip
	bounceChip = 0.5  // overshoot, so switching a chip on lands with a snap
	chipGrow   = 0.25 // how far past the label the overshoot may reach
)

// Chip is one option of a Choice or a Toggles: a word in a Row that is either
// on or off. Turning it on grows a bar out of the middle of the label and
// springs it past the width it rests at, so the switch lands hard.
//
// A chip does not take any keys itself. Enter belongs to the row it is in,
// which is what decides what being on means: one of many for a Choice, any
// number for a Toggles.
//
// Chips register animated values, create them with NewChip and do not copy
// them.
type Chip struct {
	AnimatedRect

	label *Text
	on    bool

	// fill is how far the bar has grown under the label, 0 to 1. It springs
	// past 1, and the bar is capped a little wider than the label rather
	// than clamped, so the overshoot is what is seen.
	fill animation.Value
}

// NewChip returns a chip that is off, labelled s.
func NewChip(s string) *Chip {
	p := &Chip{label: NewText(s, nil)}
	p.label.Center = true
	p.InitRect(RectF{W: float32(p.label.ContentWidth()), H: RowHeight}, SpeedLayout)
	p.fill.Init(0, springChip)
	p.fill.Spring(springChip, bounceChip)
	p.place()
	return p
}

// Label returns the Text that draws the chip, to change its font.
func (p *Chip) Label() *Text { return p.label }

// SetLabel replaces the label.
func (p *Chip) SetLabel(s string) { p.label.SetText(s) }

// On reports whether the chip is on.
func (p *Chip) On() bool { return p.on }

// SetOn switches the chip, the bar growing or shrinking into place.
func (p *Chip) SetOn(on bool) {
	if on == p.on {
		return
	}
	p.on = on
	p.fill.Target = 0
	if on {
		p.fill.Target = 1
	}
}

// Size is as wide as the label and one line high. The Row adds its own Pad on
// either side and gives the chip its height.
func (p *Chip) Size() (w, h float32) { return p.label.Size() }

// Selectable reports true: a chip is there to be switched.
func (p *Chip) Selectable() bool { return true }

func (p *Chip) setLiveFocus(a Axis) { p.label.LiveFocus = a }

// FocusRect is the label's, so the cursor hugs the word rather than the
// padded box around it, and the bar underneath lines up with it.
func (p *Chip) FocusRect() RectF { return p.label.FocusRect() }

// Release unregisters the chip's animated values.
func (p *Chip) Release() {
	p.AnimatedRect.Release()
	p.fill.Release()
	p.label.Release()
}

// Snap finishes the chip's animations, the bar included.
func (p *Chip) Snap() {
	p.AnimatedRect.Snap()
	p.fill.Snap()
	p.place()
}

// place puts the label over the whole chip, where it centres itself. The
// label is carried along with the frame rather than springing after it, so
// the two never drift apart.
func (p *Chip) place() {
	t, c := p.Target(), p.Current()
	p.label.SetTarget(t)
	p.label.X.Current = c.X
	p.label.Y.Current = c.Y
	p.label.W.Current = c.W
	p.label.H.Current = c.H
}

func (p *Chip) Update(dt float32) {
	p.place()
	p.label.Update(dt)
}

func (p *Chip) Draw(c Canvas) {
	p.label.Draw(c)

	r := p.Rect()
	full := float32(p.label.ContentWidth())
	w := min(animation.Round(p.fill.Current*full), animation.Round(full*(1+chipGrow)), r.W)
	if w <= 0 {
		return
	}
	c.FillRect(r.X+(r.W-w)/2, r.Y+r.H-chipMarkH, w, chipMarkH)
}

// Choice is a horizontal row of chips of which exactly one is on: a
// difficulty, a playback rate, a sort order. Left and right move the cursor
// over the options and enter switches to the one under it, the bar sliding
// from the old option to the new one.
//
// It is a Row, so it is also a ListItem: it goes in a List as one of its
// lines, and up and down leave it for the lines around it. Left at the first
// option is not the row's either, so on a page that goes back.
//
// Choices register animated values, create them with NewChoice and do not
// copy them.
type Choice struct {
	// The row is held by pointer, not embedded by value: it owns animated
	// values registered with the manager, which must not be copied.
	*Row

	// OnChange is called with the new index when enter is released on an
	// option that was not already on, after the switch. Choosing in code
	// does not call it.
	OnChange func(i int)

	chips  []*Chip
	chosen int
}

// NewChoice returns a row of labels with the first one on.
func NewChoice(labels ...string) *Choice {
	c := &Choice{Row: NewRow()}
	c.chips = make([]*Chip, len(labels))
	for i, s := range labels {
		c.chips[i] = NewChip(s)
		c.Add(c.chips[i])
	}
	if len(c.chips) > 0 {
		c.chips[0].SetOn(true)
	}
	return c
}

// Len returns the number of options.
func (c *Choice) Len() int { return len(c.chips) }

// Chip returns option i, to relabel it.
func (c *Choice) Chip(i int) *Chip { return c.chips[i] }

// Chosen returns the index of the option that is on, or -1 while there are no
// options.
func (c *Choice) Chosen() int {
	if len(c.chips) == 0 {
		return -1
	}
	return c.chosen
}

// Choose switches to option i, clamped to the options there are. It does not
// call OnChange, and it does not move the cursor: call Select for that.
func (c *Choice) Choose(i int) {
	if len(c.chips) == 0 {
		return
	}
	i = min(max(i, 0), len(c.chips)-1)
	if i == c.chosen {
		return
	}
	c.chips[c.chosen].SetOn(false)
	c.chosen = i
	c.chips[i].SetOn(true)
}

// HandleEvent switches to the option under the cursor when enter is released,
// squeezing the cursor while it is held. The other keys are the Row's.
func (c *Choice) HandleEvent(e Event) bool {
	if e.Key != KeyEnter {
		return c.Row.HandleEvent(e)
	}
	if click(c.Cursor, e) {
		if i := c.Selected(); i >= 0 && i != c.chosen {
			c.Choose(i)
			if c.OnChange != nil {
				c.OnChange(i)
			}
		}
	}
	return true
}

// Toggles is a horizontal row of chips, any number of which are on: the mods
// of a map, say, where HD and DT are both in. Left and right move the cursor
// over them and enter switches the one under it on or off.
//
// It is a Row, so it is also a ListItem: it goes in a List as one of its
// lines, and up and down leave it for the lines around it. Left at the first
// chip is not the row's either, so on a page that goes back.
//
// Toggles register animated values, create them with NewToggles and do not
// copy them.
type Toggles struct {
	// The row is held by pointer, not embedded by value: it owns animated
	// values registered with the manager, which must not be copied.
	*Row

	// OnChange is called with the index and its new state when enter is
	// released, after the switch. Switching in code does not call it.
	OnChange func(i int, on bool)

	chips []*Chip
}

// NewToggles returns a row of labels with all of them off.
func NewToggles(labels ...string) *Toggles {
	t := &Toggles{Row: NewRow()}
	t.chips = make([]*Chip, len(labels))
	for i, s := range labels {
		t.chips[i] = NewChip(s)
		t.Add(t.chips[i])
	}
	return t
}

// Len returns the number of chips.
func (t *Toggles) Len() int { return len(t.chips) }

// Chip returns chip i, to relabel it.
func (t *Toggles) Chip(i int) *Chip { return t.chips[i] }

// On reports whether chip i is on.
func (t *Toggles) On(i int) bool { return t.chips[i].On() }

// SetOn switches chip i. It does not call OnChange.
func (t *Toggles) SetOn(i int, on bool) { t.chips[i].SetOn(on) }

// Toggle flips chip i and calls OnChange with its new state.
func (t *Toggles) Toggle(i int) {
	c := t.chips[i]
	c.SetOn(!c.On())
	if t.OnChange != nil {
		t.OnChange(i, c.On())
	}
}

// HandleEvent switches the chip under the cursor when enter is released,
// squeezing the cursor while it is held. The other keys are the Row's.
func (t *Toggles) HandleEvent(e Event) bool {
	if e.Key != KeyEnter {
		return t.Row.HandleEvent(e)
	}
	if click(t.Cursor, e) {
		if i := t.Selected(); i >= 0 {
			t.Toggle(i)
		}
	}
	return true
}
