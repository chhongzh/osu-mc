package ui

import "osu-mc/internal/animation"

// Optional page hooks, see Viewer.
type (
	// Enterer is told when it starts flying in: resume is false when it was
	// just pushed and true when the page above it was popped.
	Enterer interface{ OnEnter(resume bool) }

	// Leaver is told when it starts flying out: exit is true when it was
	// popped and false when another page was pushed over it.
	Leaver interface{ OnLeave(exit bool) }

	// Releaser is called once a popped page has flown out, to unregister
	// its animated values.
	Releaser interface{ Release() }
)

// PageState is where a page of a Viewer is in its life.
//
//	            push over it               pop above it
//	Entering ──> Active ──> Suspending ──> Suspended ──> Entering
//	    │           │                          │
//	    └───────────┴──> Exiting (popped) <────┘ (PopToRoot)
type PageState uint8

const (
	PageEntering   PageState = iota // flying in
	PageActive                      // on screen, at rest
	PageSuspending                  // flying out to the left, under a new page
	PageSuspended                   // off screen, neither updated nor drawn
	PageExiting                     // flying out to the right, then released
)

func (s PageState) String() string {
	switch s {
	case PageEntering:
		return "entering"
	case PageActive:
		return "active"
	case PageSuspending:
		return "suspending"
	case PageSuspended:
		return "suspended"
	case PageExiting:
		return "exiting"
	}
	return "?"
}

type page struct {
	Element
	state PageState
	x     animation.Value // horizontal offset of the whole page
}

// Viewer is a stack of pages, like osu!'s ScreenStack. Push flies a new page
// in from the right while the old one flies out to the left; Pop flies the
// top page out to the right and brings the one below back from the left.
//
// Pages are plain Elements laid out for the viewer's area at x 0; the viewer
// moves them by translating the canvas. They may implement Handler,
// Container, Enterer, Leaver and Releaser. The viewer is a Container itself:
// events are routed to the top page.
//
// Going back is the viewer's own: KeyBack is intercepted before the page
// sees it, and KeyLeft goes back if the page does not use it. While the key
// is down the page peeks to the right, on release it flies out.
//
// The shared Cursor is not moved with the pages. It is drawn at the resting
// place of whatever it is focused on, so when a new page takes the focus the
// cursor leaps straight to its spot and the page flies in under it.
//
// The viewer updates the cursor itself; do not also put it in a Group.
type Viewer struct {
	// Width is how far pages fly. Width and Height are the size of a
	// page, for pages the viewer's content pushes on its own, like the
	// keyboard of an Input.
	Width, Height float32

	// Speed and Damping are the spring of the page offsets.
	Speed, Damping float32

	Cursor *Cursor

	stack   []*page
	leaving []*page // popped, still flying out
	peeking *page   // leaning right while a back key is down
}

// backLean is how far the page peeks while a back key is held.
const backLean = 8

// NewViewer returns an empty viewer whose pages fly across width pixels. Its
// Height starts at half the width, the shape of a 128x64 screen.
func NewViewer(width float32) *Viewer {
	return &Viewer{
		Width:   width,
		Height:  width / 2,
		Speed:   SpringPage,
		Damping: BouncePage,
		Cursor:  DefaultCursor,
	}
}

// Len returns the number of pages on the stack, not counting those still
// flying out.
func (v *Viewer) Len() int {
	return len(v.stack)
}

// Top returns the page on top, or nil.
func (v *Viewer) Top() Element {
	if p := v.top(); p != nil {
		return p.Element
	}
	return nil
}

// State returns the state of e, and false if e is not in the viewer.
func (v *Viewer) State(e Element) (PageState, bool) {
	for _, p := range v.stack {
		if p.Element == e {
			return p.state, true
		}
	}
	for _, p := range v.leaving {
		if p.Element == e {
			return p.state, true
		}
	}
	return 0, false
}

func (v *Viewer) top() *page {
	if len(v.stack) == 0 {
		return nil
	}
	return v.stack[len(v.stack)-1]
}

// Push puts e on top. It flies in from the right, even as the first page.
func (v *Viewer) Push(e Element) {
	if p := v.top(); p != nil {
		v.leap()
		p.state = PageSuspending
		p.x.Target = -v.Width
		if l, ok := p.Element.(Leaver); ok {
			l.OnLeave(false)
		}
	}

	p := &page{Element: e, state: PageEntering}
	p.x.Init(v.Width, v.Speed)
	p.x.Spring(v.Speed, v.Damping)
	p.x.Target = 0
	v.stack = append(v.stack, p)
	if en, ok := e.(Enterer); ok {
		en.OnEnter(false)
	}
}

// Pop flies the top page out and brings back the one below. The last page
// is never popped; Pop reports whether anything happened.
func (v *Viewer) Pop() bool {
	if len(v.stack) < 2 {
		return false
	}
	v.leap()
	v.exit(v.pop())
	v.resume(v.top())
	return true
}

// Back pops the top page. On the root page, where there is nothing to go
// back to, the page and the cursor give a little shove instead. It has no
// result so it can be used as a callback: list.OnLeft = viewer.Back.
func (v *Viewer) Back() {
	if v.Pop() {
		return
	}
	if p := v.top(); p != nil {
		p.x.Kick(backNudge)
		if v.Cursor != nil {
			v.Cursor.Nudge(backNudge)
		}
	}
}

// backNudge is how far the root page gives when there is no going back.
const backNudge = 6

func (v *Viewer) leap() {
	if v.Cursor != nil {
		v.Cursor.Leap()
	}
}

// PopToRoot pops every page but the first. Only the top one is seen flying
// out; the ones in between are off screen and are released at once.
func (v *Viewer) PopToRoot() bool {
	if len(v.stack) < 2 {
		return false
	}
	v.leap()
	top := v.pop()
	for len(v.stack) > 1 {
		p := v.pop()
		if l, ok := p.Element.(Leaver); ok {
			l.OnLeave(true)
		}
		v.release(p)
	}
	v.exit(top)
	v.resume(v.top())
	return true
}

func (v *Viewer) pop() *page {
	n := len(v.stack) - 1
	p := v.stack[n]
	v.stack[n] = nil
	v.stack = v.stack[:n]
	return p
}

func (v *Viewer) exit(p *page) {
	p.state = PageExiting
	p.x.Target = v.Width
	v.leaving = append(v.leaving, p)
	if l, ok := p.Element.(Leaver); ok {
		l.OnLeave(true)
	}
}

func (v *Viewer) resume(p *page) {
	p.state = PageEntering
	p.x.Target = 0
	if en, ok := p.Element.(Enterer); ok {
		en.OnEnter(true)
	}
}

func (v *Viewer) release(p *page) {
	p.x.Release()
	if r, ok := p.Element.(Releaser); ok {
		r.Release()
	}
}

// Focused returns the top page, where events go.
func (v *Viewer) Focused() Element {
	return v.Top()
}

// InterceptEvent takes KeyBack before the page can.
func (v *Viewer) InterceptEvent(e Event) bool {
	return e.Key == KeyBack && v.back(e)
}

// HandleEvent takes KeyLeft when the page did not.
func (v *Viewer) HandleEvent(e Event) bool {
	return e.Key == KeyLeft && v.back(e)
}

func (v *Viewer) back(e Event) bool {
	switch e.Phase {
	case Press:
		p := v.top()
		if p == nil {
			return false
		}
		// The root peeks only half as far: there is nothing behind it.
		lean := float32(backLean)
		if len(v.stack) < 2 {
			lean /= 2
		}
		v.peek(p, lean)
	case Release:
		if p := v.peeking; p != nil {
			v.peek(nil, 0)
			p.x.Target = 0
		}
		v.Back()
	}
	return true
}

func (v *Viewer) peek(p *page, dx float32) {
	v.peeking = p
	if p != nil {
		p.x.Target = dx
	}
	if v.Cursor != nil {
		v.Cursor.Lean(dx, 0)
	}
}

// Update advances the state machine and updates every page that can be seen.
// A transition ends when the page's spring has come to rest.
func (v *Viewer) Update(dt float32) {
	for _, p := range v.stack {
		switch p.state {
		case PageEntering:
			if p.x.Settled() {
				p.state = PageActive
			}
		case PageSuspending:
			if p.x.Settled() {
				p.state = PageSuspended
			}
		}
		if p.state != PageSuspended {
			p.Update(dt)
		}
	}

	n := 0
	for _, p := range v.leaving {
		if p.x.Settled() {
			v.release(p)
			continue
		}
		p.Update(dt)
		v.leaving[n] = p
		n++
	}
	clear(v.leaving[n:])
	v.leaving = v.leaving[:n]

	if v.Cursor != nil {
		v.Cursor.Update(dt)
	}
}

func (v *Viewer) Draw(c Canvas) {
	for _, p := range v.stack {
		if p.state != PageSuspended {
			v.draw(c, p)
		}
	}
	for _, p := range v.leaving {
		v.draw(c, p)
	}
	if v.Cursor != nil {
		v.Cursor.Draw(c)
	}
}

func (v *Viewer) draw(c Canvas, p *page) {
	x := p.x.Int()
	if w := int16(v.Width); x >= w || x <= -w {
		return
	}
	p.Draw(c.Translate(x, 0))
}
