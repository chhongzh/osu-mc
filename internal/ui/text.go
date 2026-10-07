package ui

import (
	"osu-mc/internal/animation"

	"tinygo.org/x/tinyfont"
	"tinygo.org/x/tinyfont/notosans"
)

// DefaultFont is used by texts created without a font.
var DefaultFont = &notosans.Notosans12pt

// Axis is a set of directions.
type Axis uint8

const (
	AxisX Axis = 1 << iota
	AxisY
)

// Marquee phases.
const (
	marqueeHoldStart = iota
	marqueeForward
	marqueeHoldEnd
	marqueeReturn
)

// Text is a single line of text. It never wraps: whatever does not fit in W is
// clipped, and the line can be scrolled horizontally to bring it into view.
//
// Texts register animated values, create them with NewText and do not copy
// them.
type Text struct {
	AnimatedRect

	Font *tinyfont.Font

	// MarqueeSpeed is how fast the marquee scrolls forward, in px/s.
	// MarqueePause is how long it rests at either end, in seconds.
	MarqueeSpeed float32
	MarqueePause float32

	// LiveFocus makes the cursor follow where the text is right now on
	// these axes, rather than where it is heading, so it rides along with
	// the text's own motion. Centred lists set it: there the selection's
	// target hardly moves, the items move under it.
	LiveFocus Axis

	// Center centres a line that is narrower than the rectangle.
	Center bool

	// Static makes the text a label rather than an entry: a list shows it
	// but the selection passes over it, like a heading.
	Static bool

	text  string
	width int16 // sum of the glyph advances

	// scroll is how far the line is shifted left, 0..Overflow.
	scroll animation.Value

	marquee      bool
	marqueePhase uint8
	marqueeWait  float32
}

// NewText returns a text sized to fit s. A nil font means DefaultFont.
func NewText(s string, font *tinyfont.Font) *Text {
	if font == nil {
		font = DefaultFont
	}
	t := &Text{Font: font, MarqueeSpeed: 24, MarqueePause: 1}
	t.text = s
	t.width = measure(font, s)
	t.InitRect(RectF{0, 0, float32(t.width), float32(t.LineHeight())}, SpeedLayout)
	t.scroll.Init(0, SpeedScroll)
	return t
}

// Text returns the string being shown.
func (t *Text) Text() string {
	return t.text
}

// SetText replaces the string. The size is left alone; the scroll offset is
// kept if it is still in range.
func (t *Text) SetText(s string) {
	if s == t.text {
		return
	}
	t.text = s
	t.width = measure(t.Font, s)
	t.clampScroll()
	animation.Default.Invalidate()
}

// ContentWidth returns the width of the whole line in pixels.
func (t *Text) ContentWidth() int16 {
	return t.width
}

// LineHeight returns the height of the font's bounding box.
func (t *Text) LineHeight() int16 {
	return int16(t.Font.BBox[1])
}

// Overflow returns how many pixels of the line do not fit in the target
// width, which is also the largest valid scroll offset.
func (t *Text) Overflow() float32 {
	return max(0, float32(t.width)-t.W.Target)
}

// ScrollBy shifts the line by delta pixels, positive to reveal more of the
// right hand side. The target is clamped; the visible offset eases there.
func (t *Text) ScrollBy(delta float32) {
	t.ScrollTo(t.scroll.Target + delta)
}

// ScrollTo sets the scroll offset, clamped to 0..Overflow.
func (t *Text) ScrollTo(x float32) {
	t.scroll.Target = clampf(x, 0, t.Overflow())
}

// Scroll returns the current, eased, scroll offset.
func (t *Text) Scroll() float32 {
	return t.scroll.Current
}

// SetMarquee starts or stops automatic scrolling. When on and the line
// overflows, it rests, scrolls to the end at MarqueeSpeed, rests, eases back
// to the start and repeats. Stopping leaves the offset where it is.
func (t *Text) SetMarquee(on bool) {
	if on == t.marquee {
		return
	}
	t.marquee = on
	t.marqueePhase = marqueeHoldStart
	t.marqueeWait = t.MarqueePause
}

// Marquee reports whether automatic scrolling is on.
func (t *Text) Marquee() bool {
	return t.marquee
}

// Size is the size of the line: as wide as the text, one line high.
func (t *Text) Size() (w, h float32) {
	return float32(t.width), float32(t.LineHeight())
}

// Selectable reports whether a list may select the text, which it may
// unless it is Static.
func (t *Text) Selectable() bool { return !t.Static }

func (t *Text) setLiveFocus(a Axis) { t.LiveFocus = a }

// Release unregisters the text's animated values. Call it when the text is
// thrown away.
func (t *Text) Release() {
	t.AnimatedRect.Release()
	t.scroll.Release()
}

// FocusRect is the area the cursor covers: the visible part of the line.
func (t *Text) FocusRect() RectF {
	r := t.Target()
	if t.LiveFocus&AxisX != 0 {
		r.X = t.X.Current
	}
	if t.LiveFocus&AxisY != 0 {
		r.Y = t.Y.Current
	}
	if w := float32(t.width); w < r.W {
		if t.Center {
			r.X += (r.W - w) / 2
		}
		r.W = w
	}
	return r
}

func (t *Text) Update(dt float32) {
	// The width may have changed underneath us.
	t.clampScroll()

	if !t.marquee || t.Overflow() == 0 {
		return
	}
	switch t.marqueePhase {
	case marqueeHoldStart, marqueeHoldEnd:
		t.marqueeWait -= dt
		if t.marqueeWait > 0 {
			return
		}
		if t.marqueePhase == marqueeHoldStart {
			t.marqueePhase = marqueeForward
		} else {
			t.ScrollTo(0)
			t.marqueePhase = marqueeReturn
		}
	case marqueeForward:
		// The target moves at a constant speed and the offset eases after
		// it, which reads as a steady scroll with soft start and stop.
		t.ScrollBy(t.MarqueeSpeed * dt)
		if t.scroll.Target >= t.Overflow() {
			t.marqueePhase = marqueeHoldEnd
			t.marqueeWait = t.MarqueePause
		}
	case marqueeReturn:
		if t.scroll.Current < 0.5 {
			t.marqueePhase = marqueeHoldStart
			t.marqueeWait = t.MarqueePause
		}
	}
}

func (t *Text) Draw(c Canvas) {
	r := t.Rect()
	c = c.Within(r)
	if c.Empty() {
		return
	}

	// Centre the font's bounding box vertically in the rectangle.
	baseline := r.Y + (r.H-t.LineHeight())/2 - int16(t.Font.BBox[3])
	x := r.X - t.scroll.Int()
	if t.Center && t.width < r.W {
		x += (r.W - t.width) / 2
	}
	clip := c.Clip()
	right := clip.X + clip.W
	for _, ch := range t.text {
		if x >= right {
			break
		}
		g := glyph(t.Font, ch)
		c.DrawGlyph(g, x, baseline)
		x += int16(g.XAdvance)
	}
}

func (t *Text) clampScroll() {
	if ov := t.Overflow(); t.scroll.Target > ov {
		t.scroll.Target = ov
	}
}

// glyph looks up r. tinyfont.Font returns a pointer into its glyph table (or
// to its placeholder glyph), so this does not allocate.
func glyph(f *tinyfont.Font, r rune) *tinyfont.Glyph {
	return f.GetGlyph(r).(*tinyfont.Glyph)
}

func measure(f *tinyfont.Font, s string) int16 {
	var w int16
	for _, r := range s {
		w += int16(glyph(f, r).XAdvance)
	}
	return w
}
