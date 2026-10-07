package ui

import (
	"osu-mc/internal/animation"

	"tinygo.org/x/tinyfont"
)

// TextView geometry and motion.
const (
	viewPad     = 2            // space left of the text
	viewReserve = barWideW + 1 // space right of the text, for the scrollbar
	viewStepX   = 16           // how far left and right scroll a raw line, in px
	viewCascade = 10           // how far the text trails as the page flies in
	viewTab     = ' '          // a tab is drawn as this
)

// The scroll offsets spring like list items, but critically damped: text
// that overshoots and swings back is hard to read on a 1 bit screen.
const (
	springView = SpringItem
	bounceView = 1
)

// textLine is one line of a TextView: a slice of its text and its width.
type textLine struct {
	start, end int
	width      int16
}

// TextView shows a long text over a whole page and lets the keys move the
// viewport over it, like a reader.
//
// With Wrap on, paragraphs are broken into lines at spaces to fit the
// width, and a word too long for a line is broken where it overflows. With
// Wrap off, only '\n' starts a new line and long lines run off to the right.
//
// Up and down scroll by a line. Left and right scroll sideways by viewStepX
// when the text does not wrap; when it wraps there is nothing to the side,
// so they page up and down instead. Held against an end, the text is pulled
// that way until the key is let go, like a List. Enter toggles Wrap. The
// view takes no back key: in a Viewer, KeyBack goes back.
//
// The view is a page and covers its whole rectangle, so it hides the shared
// cursor while it is on screen; the page below takes it back when it
// returns.
//
// TextViews register animated values, create them with NewTextView and do
// not copy them.
type TextView struct {
	AnimatedRect

	Font *tinyfont.Font

	// Cursor is the shared cursor, hidden while the view is on screen.
	Cursor *Cursor

	// OnWrap is called after enter has toggled the wrapping.
	OnWrap func(wrap bool)

	text    string
	wrap    bool
	lines   []textLine
	widest  int16 // width of the widest line
	brokeAt int16 // the width the lines were broken for, while wrapping
	dirty   bool  // the lines must be broken again
	laidOut bool

	// The viewport offset, and the pull of the text past either end. The
	// layout only ever moves their targets.
	scrollX, scrollY animation.Value
	pullX, pullY     animation.Value

	barV, barH AnimatedRect
	barIdle    float32
}

// NewTextView returns a view of s, wrapped or not. A nil font means
// DefaultFont.
func NewTextView(s string, font *tinyfont.Font, wrap bool) *TextView {
	if font == nil {
		font = DefaultFont
	}
	v := &TextView{
		Font:    font,
		Cursor:  DefaultCursor,
		text:    s,
		wrap:    wrap,
		dirty:   true,
		barIdle: barIdleTime,
	}
	v.InitRect(RectF{}, SpeedLayout)
	v.scrollX.Init(0, springView)
	v.scrollX.Spring(springView, bounceView)
	v.scrollY.Init(0, springView)
	v.scrollY.Spring(springView, bounceView)
	initPull(&v.pullX)
	initPull(&v.pullY)
	v.barV.InitRect(RectF{}, SpringBar)
	v.barV.SetSpring(SpringBar, BounceBar)
	v.barH.InitRect(RectF{}, SpringBar)
	v.barH.SetSpring(SpringBar, BounceBar)
	return v
}

// Text returns the text being shown.
func (v *TextView) Text() string { return v.text }

// SetText replaces the text. The line that was at the top of the viewport
// stays there as far as the new text goes, so text appended at the end does
// not move what is being read.
func (v *TextView) SetText(s string) {
	if s == v.text {
		return
	}
	v.text = s
	v.dirty = true
	v.Layout()
}

// Wrap reports whether the text is broken into lines to fit the width.
func (v *TextView) Wrap() bool { return v.wrap }

// SetWrap turns wrapping on or off. The text reflows around the line at the
// top of the viewport, which stays where it is.
func (v *TextView) SetWrap(on bool) {
	if on == v.wrap {
		return
	}
	v.wrap = on
	v.dirty = true
	v.Layout()
}

// Lines returns the number of lines the text is laid out in.
func (v *TextView) Lines() int { return len(v.lines) }

// Line returns line i, without its line break.
func (v *TextView) Line(i int) string {
	l := v.lines[i]
	return v.text[l.start:l.end]
}

// LineHeight returns the distance between baselines.
func (v *TextView) LineHeight() int16 {
	if v.Font.YAdvance > 0 {
		return int16(v.Font.YAdvance)
	}
	return int16(v.Font.BBox[1])
}

// ascent is how far the font reaches above its baseline.
func (v *TextView) ascent() int16 {
	return -int16(v.Font.BBox[3])
}

// Scroll returns where the viewport is heading, in pixels from the top left
// of the text.
func (v *TextView) Scroll() (x, y float32) {
	return v.scrollX.Target, v.scrollY.Target
}

// textWidth returns the width the text can be shown in, between the
// padding and the scrollbar.
func (v *TextView) textWidth() int16 {
	return int16(v.W.Target) - viewPad - viewReserve
}

// contentHeight is how far the text reaches down: every line but the last
// one line apart, and the last one with its descenders.
func (v *TextView) contentHeight() float32 {
	if len(v.lines) == 0 {
		return 0
	}
	return float32(int16(len(v.lines)-1)*v.LineHeight() + int16(v.Font.BBox[1]))
}

// maxScroll returns the largest scroll offsets along both axes.
func (v *TextView) maxScroll() (x, y float32) {
	if !v.wrap {
		x = max(0, float32(v.widest-v.textWidth()))
	}
	return x, max(0, v.contentHeight()-v.H.Target)
}

// Layout breaks the text into lines if it changed or, while wrapping, the
// width did, keeps the scroll targets in range and places the scrollbars.
// Update calls it every frame; it allocates only when the lines change.
func (v *TextView) Layout() {
	v.laidOut = true
	w := v.textWidth()
	if v.dirty || v.wrap && w != v.brokeAt {
		v.reflow(w)
	}

	mx, my := v.maxScroll()
	v.scrollX.Target = clampf(v.scrollX.Target, 0, mx)
	v.scrollY.Target = clampf(v.scrollY.Target, 0, my)

	t := v.Target()
	bw := float32(barThinW)
	if v.barIdle < barIdleTime {
		bw = barWideW
	}
	if my > 0 {
		content := v.contentHeight()
		h := max(t.H*t.H/content, barMinH)
		v.barV.SetTarget(RectF{
			X: t.X + t.W - bw,
			Y: t.Y + v.scrollY.Target/my*(t.H-h),
			W: bw,
			H: h,
		})
	} else {
		v.barV.SetTarget(RectF{t.X + t.W, t.Y, 0, t.H})
	}
	if mx > 0 {
		tw := float32(v.textWidth())
		w := max(tw*tw/float32(v.widest), barMinH)
		v.barH.SetTarget(RectF{
			X: t.X + viewPad + v.scrollX.Target/mx*(tw-w),
			Y: t.Y + t.H - 1,
			W: w,
			H: 1,
		})
	} else {
		v.barH.SetTarget(RectF{t.X + viewPad, t.Y + t.H, t.W - viewPad - viewReserve, 0})
	}
}

// reflow breaks the text into lines for width w. The line at the top of the
// viewport is the anchor: the scroll offset shifts by however far it moved.
// The lines themselves land in their new places at once, so the offset is
// shifted with them, current and target alike, like a Jump that keeps the
// motion: the anchored line stays put and a scroll in progress carries on.
func (v *TextView) reflow(w int16) {
	anchor, frac := 0, float32(0)
	if lh := float32(v.LineHeight()); len(v.lines) > 0 {
		top := min(int(v.scrollY.Target/lh), len(v.lines)-1)
		anchor = v.lines[top].start
		frac = v.scrollY.Target - float32(top)*lh
	}

	v.dirty = false
	v.brokeAt = w
	v.lines = v.lines[:0]
	v.widest = 0
	for p := 0; ; {
		end := p
		for end < len(v.text) && v.text[end] != '\n' {
			end++
		}
		if v.wrap && w > 0 {
			v.breakParagraph(p, end, w)
		} else {
			v.emit(p, end)
		}
		if end >= len(v.text) {
			break
		}
		p = end + 1
	}

	// The anchor is on the last line that starts at or before it.
	top := 0
	for i, l := range v.lines {
		if l.start > anchor {
			break
		}
		top = i
	}
	d := float32(top)*float32(v.LineHeight()) + frac - v.scrollY.Target
	v.scrollY.Current += d
	v.scrollY.Target += d
	animation.Default.Invalidate()
}

// breakParagraph breaks text[p0:p1], which holds no '\n', into lines no
// wider than w. A line breaks at its last space, which is dropped; a word
// with no space before it on the line breaks where it overflows. A line
// always takes at least one rune, so a w narrower than a glyph still ends.
func (v *TextView) breakParagraph(p0, p1 int, w int16) {
	start, space := p0, -1 // space: the last space on the line, or -1
	var x int16            // width of text[start:i]
	for o, r := range v.text[p0:p1] {
		i := p0 + o
		adv := v.advance(r)
		if x+adv > w && i > start {
			switch {
			case r == ' ':
				// The space itself overflows: break on it and drop it.
				v.emit(start, i)
				start, space, x = i+1, -1, 0
				continue
			case space > start:
				v.emit(start, space)
				start, space = space+1, -1
				x = v.measure(start, i)
			}
			// What is left of the word fitted on the line before,
			// but may still not fit with r.
			if x+adv > w && i > start {
				v.emit(start, i)
				start, space, x = i, -1, 0
			}
		}
		if r == ' ' {
			space = i
		}
		x += adv
	}
	v.emit(start, p1)
}

func (v *TextView) emit(start, end int) {
	w := v.measure(start, end)
	v.lines = append(v.lines, textLine{start, end, w})
	v.widest = max(v.widest, w)
}

func (v *TextView) measure(start, end int) int16 {
	var w int16
	for _, r := range v.text[start:end] {
		w += v.advance(r)
	}
	return w
}

// advance returns how far r moves the pen. A carriage return, of a "\r\n"
// line break, takes no room.
func (v *TextView) advance(r rune) int16 {
	switch r {
	case '\r':
		return 0
	case '\t':
		r = viewTab
	}
	return int16(glyph(v.Font, r).XAdvance)
}

// HandleEvent scrolls with the arrow keys and toggles the wrapping on enter.
//
// The scroll happens on release and on every repeat, like moving the
// selection of a list; the press of a key that is blocked by an end pulls
// the text there instead.
func (v *TextView) HandleEvent(e Event) bool {
	lh := float32(v.LineHeight())
	// A page keeps one line of the previous page in sight.
	page := max(lh, v.H.Target-lh)
	switch e.Key {
	case KeyUp:
		v.scroll(e, &v.scrollY, &v.pullY, -lh, false)
	case KeyDown:
		v.scroll(e, &v.scrollY, &v.pullY, lh, false)
	case KeyLeft:
		if v.wrap {
			v.scroll(e, &v.scrollY, &v.pullY, -page, false)
		} else {
			v.scroll(e, &v.scrollX, &v.pullX, -viewStepX, true)
		}
	case KeyRight:
		if v.wrap {
			v.scroll(e, &v.scrollY, &v.pullY, page, false)
		} else {
			v.scroll(e, &v.scrollX, &v.pullX, viewStepX, true)
		}
	case KeyEnter:
		if e.Phase == Release {
			v.SetWrap(!v.wrap)
			if v.OnWrap != nil {
				v.OnWrap(v.wrap)
			}
		}
	default:
		return false
	}
	return true
}

// scroll is the press, repeat and release of a key that moves offset s by
// delta. There is no cursor to lean, so unlike seq.move the text itself is
// the only feedback: on both axes the pull goes against the key, so held
// against an end the text visibly retreats from that edge rather than
// sliding further past it.
func (v *TextView) scroll(e Event, s, pull *animation.Value, delta float32, horizontal bool) {
	mx, my := v.maxScroll()
	end := my
	if horizontal {
		end = mx
	}
	blocked := delta < 0 && s.Target <= 0 || delta > 0 && s.Target >= end

	dir := float32(1)
	if delta < 0 {
		dir = -1
	}
	var dx, dy, pdx, pdy float32
	if horizontal {
		dx, pdx = dir, -dir
	} else {
		dy, pdy = dir, -dir
	}
	// The cursor is hidden, the text is all the feedback there is.
	if push(nil, pull, e, blocked, dx, dy, pdx, pdy) && !blocked {
		s.Target = clampf(s.Target+delta, 0, end)
		v.barIdle = 0
	}
}

// OnEnter is called by a Viewer when the view starts flying in. The text
// trails behind the page, and the cursor goes away.
func (v *TextView) OnEnter(resume bool) {
	if !v.laidOut {
		v.Snap()
	}
	v.pullX.Current += enterSide(resume) * viewCascade
	if v.Cursor != nil {
		v.Cursor.Focus(nil)
	}
}

// OnLeave is called by a Viewer when the view starts flying out. The text
// is thrown against the direction of travel, so it lags behind.
func (v *TextView) OnLeave(exit bool) {
	v.pullX.Kick(leaveKick(exit) * viewCascade)
}

// Release unregisters the view's animated values. A Viewer calls it once
// the view has flown out.
func (v *TextView) Release() {
	v.AnimatedRect.Release()
	v.scrollX.Release()
	v.scrollY.Release()
	v.pullX.Release()
	v.pullY.Release()
	v.barV.Release()
	v.barH.Release()
}

// Snap lays the view out and finishes all running animations.
func (v *TextView) Snap() {
	v.AnimatedRect.Snap()
	v.Layout()
	v.scrollX.Snap()
	v.scrollY.Snap()
	v.pullX.Snap()
	v.pullY.Snap()
	v.barV.Snap()
	v.barH.Snap()
}

func (v *TextView) Update(dt float32) {
	v.barIdle += dt
	v.Layout()
}

func (v *TextView) Draw(c Canvas) {
	r := v.Rect()
	c = c.Within(r)
	if c.Empty() {
		return
	}

	// The text is drawn pulled and clipped short of the scrollbar, which
	// is not pulled.
	tc := c.Within(Rect{r.X, r.Y, r.W - viewReserve, r.H})
	if !tc.Empty() {
		v.drawLines(tc, r)
	}
	if b := v.barV.Rect(); !b.Empty() {
		c.FillRect(b.X, b.Y, b.W, b.H)
	}
	if b := v.barH.Rect(); !b.Empty() {
		c.FillRect(b.X, b.Y, b.W, b.H)
	}
}

// drawLines draws the lines that show in c, for the view at r.
func (v *TextView) drawLines(c Canvas, r Rect) {
	lh := v.LineHeight()
	x0 := r.X + viewPad + animation.Round(v.pullX.Current-v.scrollX.Current)
	y0 := r.Y + animation.Round(v.pullY.Current-v.scrollY.Current)
	clip := c.Clip()
	right, bottom := clip.X+clip.W, clip.Y+clip.H

	// Start one line above the first one whose top is in sight: its
	// descenders may reach down into it.
	first := 0
	if d := clip.Y - y0; d > 0 {
		first = max(0, int(d/lh)-1)
	}
	asc := v.ascent()
	for i := first; i < len(v.lines); i++ {
		top := y0 + int16(i)*lh
		if top >= bottom {
			break
		}
		l := v.lines[i]
		x := x0
		for _, ch := range v.text[l.start:l.end] {
			if x >= right {
				break
			}
			switch ch {
			case '\r':
				continue
			case '\t':
				ch = viewTab
			}
			g := glyph(v.Font, ch)
			c.DrawGlyph(g, x, top+asc)
			x += int16(g.XAdvance)
		}
	}
}
