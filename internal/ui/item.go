package ui

import "osu-mc/internal/animation"

// ListItem is one entry of a List or a Row.
//
// The screen is too small for anything but lists, so every control is a
// list item: a line of text, a text field, a row of chips, a whole list.
// The list places its items, moves the selection over them and the cursor
// onto them; an item draws itself where it is put and handles the keys that
// are its own.
//
// What else an item does comes from the interfaces it implements as well:
// a Focusable is where the cursor goes when the item is selected, a Focuser
// puts the cursor somewhere itself (on its own selection), a Seeker does so
// from where the focus comes from, a Handler takes the keys it wants before
// the list does and a Container passes them on to its own selection.
type ListItem interface {
	Element

	// Frame is the rectangle the list places the item with: the list sets
	// its target every frame, the item draws itself at its current value.
	Frame() *AnimatedRect

	// Size is the size the item asks for. A List takes the height and a
	// Row the width; the other side is the list's.
	Size() (w, h float32)

	// Selectable reports whether the selection may stop on the item. A
	// heading may not, the list skips it.
	Selectable() bool

	// Snap finishes the item's animations. Release unregisters its
	// animated values; the list calls it once the item has been removed
	// and is out of sight, or when the list itself is released.
	Snap()
	Release()
}

// Seeker is an item that can choose what to focus from where the focus is
// coming from, see Row.FocusNear.
type Seeker interface {
	FocusNear(from RectF)
}

// holder is an item that holds other focusable things, so that a list can
// tell whether the cursor is inside it.
type holder interface {
	holds(f Focusable) bool
}

// liveFocuser is an item that can make the cursor follow it as it moves
// rather than where it is heading, see Text.LiveFocus.
type liveFocuser interface {
	setLiveFocus(a Axis)
}

// marqueer is an item that scrolls while it is selected.
type marqueer interface {
	SetMarquee(on bool)
	ScrollTo(x float32)
}

// cascader is an item that trails behind its list as the list flies in or
// out of a Viewer, by dx times its own cascade distance.
type cascader interface {
	trail(dx float32)
	lag(dx float32)
}

// holds reports whether f, where a cursor is, is it or inside it.
func holds(it ListItem, f Focusable) bool {
	if f == nil {
		return false
	}
	if g, ok := it.(Focusable); ok && g == f {
		return true
	}
	h, ok := it.(holder)
	return ok && h.holds(f)
}

// seq is what List and Row have in common: the items and the selection, the
// cursor that follows it, the keys that move it, items coming and going
// while on screen, and the pull at either end. How the items are laid out
// along the axis is each type's own.
type seq struct {
	// Cursor follows the selection while it is in the list.
	Cursor *Cursor

	// OnSelect is called with the selected index when enter is released
	// on an item that does not take enter itself.
	OnSelect func(i int)

	// Pass lets the keys that move the selection through to the container
	// at these edges, instead of the content being pulled.
	Pass Edge

	// Gap is the space between items.
	Gap float32

	// ItemSpeed is the spring frequency of the selected item. Items further
	// away are slowed down by Stagger per step, so the list moves as a
	// wave. Bounce is the damping ratio of the motion along the axis: 1
	// stops dead, lower values overshoot.
	ItemSpeed float32
	Stagger   float32
	Bounce    float32

	// Centered keeps the selection in the middle of the viewport, except
	// near the ends, where the list stops at its first or last item instead
	// of leaving the screen half empty. Otherwise the list scrolls only when
	// the selection would get closer than ScrollMargin to an edge.
	Centered     bool
	ScrollMargin float32

	// Cascade is how far, per step away from the selection, the items
	// trail behind when the list flies in or out of a Viewer. Items that
	// are lists themselves trail by their own Cascade instead.
	Cascade float32

	items    []ListItem
	leaving  []ListItem // removed, closing up before they are released
	opening  []ListItem // inserted on screen, opened up by the next layout
	placeAll bool       // all items are new, placed by the next layout

	selected int // -1 while nothing can be selected, see settle
	laidOut  bool
	scroll   float32 // content offset the viewport is heading for
	content  float32 // length of the content along the axis

	pull animation.Value // offset of the items while held against an end

	vertical bool
}

func (s *seq) init(vertical bool, items []ListItem) {
	s.Cursor = DefaultCursor
	s.ItemSpeed = SpringItem
	s.Bounce = BounceScroll
	s.Centered = true
	s.ScrollMargin = 8
	s.vertical = vertical
	s.items = append([]ListItem(nil), items...)
	s.selected = s.nearest(0)
	initPull(&s.pull)
}

// Len returns the number of items.
func (s *seq) Len() int { return len(s.items) }

// Item returns item i.
func (s *seq) Item(i int) ListItem { return s.items[i] }

// Index returns the index of it, or -1.
func (s *seq) Index(it ListItem) int {
	for i, o := range s.items {
		if o == it {
			return i
		}
	}
	return -1
}

// settle selects the first item that can be selected if nothing is
// selected yet: an item may have become selectable after it was put in,
// like a row that was empty then.
func (s *seq) settle() {
	if s.selected < 0 {
		s.selected = s.nearest(0)
	}
}

// selectable reports whether any item can be selected.
func (s *seq) selectable() bool {
	s.settle()
	return s.selected >= 0
}

// Selected returns the index of the selected item, or -1 while there is
// nothing that can be selected.
func (s *seq) Selected() int {
	s.settle()
	return s.selected
}

// SelectedItem returns the selected item, or nil.
func (s *seq) SelectedItem() ListItem {
	s.settle()
	if s.selected < 0 || s.selected >= len(s.items) {
		return nil
	}
	return s.items[s.selected]
}

// Focused returns the selected item, so keys go to it before the list.
func (s *seq) Focused() Element {
	if it := s.SelectedItem(); it != nil {
		return it
	}
	return nil
}

// holds reports whether f is one of the items or inside one.
func (s *seq) holds(f Focusable) bool {
	for _, it := range s.items {
		if holds(it, f) {
			return true
		}
	}
	return false
}

// next returns the first item after from in direction dir that can be
// selected, or -1.
func (s *seq) next(from, dir int) int {
	for i := from + dir; i >= 0 && i < len(s.items); i += dir {
		if s.items[i].Selectable() {
			return i
		}
	}
	return -1
}

// nearest returns i if it can be selected, else the closest item that can,
// looking forward first; or -1.
func (s *seq) nearest(i int) int {
	i = min(max(i, 0), len(s.items)-1)
	if i < 0 || s.items[i].Selectable() {
		return i
	}
	if n := s.next(i, 1); n >= 0 {
		return n
	}
	return s.next(i, -1)
}

// Select selects item i, or the item nearest to it that can be selected. If
// the cursor was in the old selection it moves along.
func (s *seq) Select(i int) {
	i = s.nearest(i)
	if i < 0 || i == s.selected {
		return
	}
	prev := s.SelectedItem()
	s.selected = i
	if prev == nil {
		return
	}
	deselect(prev)
	if s.Cursor != nil && holds(prev, s.Cursor.Target()) {
		s.focusSelected(true)
	}
}

// Move moves the selection by delta items that can be selected, stopping at
// either end.
func (s *seq) Move(delta int) {
	s.settle()
	i, dir := s.selected, 1
	if delta < 0 {
		dir, delta = -1, -delta
	}
	for ; delta > 0; delta-- {
		n := s.next(i, dir)
		if n < 0 {
			break
		}
		i = n
	}
	s.Select(i)
}

// Focus puts the cursor on the selected item.
func (s *seq) Focus() { s.focusSelected(false) }

// focusSelected puts the cursor on the selected item. With seek, an item
// that chooses what to focus does so from where the cursor is now.
func (s *seq) focusSelected(seek bool) {
	it := s.SelectedItem()
	if it == nil {
		return
	}
	if sk, ok := it.(Seeker); ok && seek && s.Cursor != nil && s.Cursor.Target() != nil {
		sk.FocusNear(s.Cursor.Target().FocusRect())
	} else if f, ok := it.(Focuser); ok {
		f.Focus()
	} else if f, ok := it.(Focusable); ok && s.Cursor != nil {
		s.Cursor.Focus(f)
	}
}

// deselect stops a text that scrolled while selected and puts it back at
// its start.
func deselect(it ListItem) {
	if m, ok := it.(marqueer); ok {
		m.SetMarquee(false)
		m.ScrollTo(0)
	}
}

// Add appends items.
func (s *seq) Add(items ...ListItem) {
	for _, it := range items {
		s.Insert(len(s.items), it)
	}
}

// Insert puts it before item i, or at the end if i is Len. On screen it
// opens up where it lands while the items after it make room. The selected
// item stays selected; a list that had nothing to select selects it, but
// does not take the cursor, call Focus for that.
func (s *seq) Insert(i int, it ListItem) {
	i = min(max(i, 0), len(s.items))
	s.items = append(s.items, nil)
	copy(s.items[i+1:], s.items[i:])
	s.items[i] = it
	switch {
	case s.selected < 0:
		s.selected = s.nearest(i)
	case i <= s.selected:
		s.selected++
	}
	if s.laidOut {
		s.opening = append(s.opening, it)
	}
}

// Remove takes item i out. On screen it closes up and the items after it
// move into its place; it is released once it has closed, so it must not be
// used again. If it was selected, the selection moves to the item that took
// its place, or to the one before at the end, and the cursor goes with it.
func (s *seq) Remove(i int) {
	if i < 0 || i >= len(s.items) {
		return
	}
	it := s.items[i]
	copy(s.items[i:], s.items[i+1:])
	s.items[len(s.items)-1] = nil
	s.items = s.items[:len(s.items)-1]
	s.unopen(it)

	switch {
	case i < s.selected:
		s.selected--
	case i == s.selected:
		deselect(it)
		s.selected = s.nearest(i)
		if s.Cursor != nil && holds(it, s.Cursor.Target()) {
			if s.selected >= 0 {
				s.focusSelected(true)
			} else {
				s.Cursor.Focus(nil)
			}
		}
	}

	if !s.laidOut {
		it.Release()
		return
	}
	// It closes around its middle without overshooting, which would turn
	// it inside out.
	f := it.Frame()
	t := f.Target()
	if s.vertical {
		f.Y.Target, f.H.Target = t.Y+t.H/2, 0
		f.Y.Spring(s.ItemSpeed, 1)
	} else {
		f.X.Target, f.W.Target = t.X+t.W/2, 0
		f.X.Spring(s.ItemSpeed, 1)
	}
	s.leaving = append(s.leaving, it)
}

// SetItems replaces every item at once, without animation: the old items
// are released right away. The selection keeps its index as far as the new
// items go, and the cursor moves to it if it was in the list.
func (s *seq) SetItems(items ...ListItem) {
	had := s.Cursor != nil && s.holds(s.Cursor.Target())
	for _, it := range s.items {
		it.Release()
	}
	s.releaseLeaving()
	s.items = append(s.items[:0:0], items...)
	s.opening = s.opening[:0]
	s.placeAll = s.laidOut
	s.selected = s.nearest(s.selected)
	if !had {
		return
	}
	if s.selected >= 0 {
		s.focusSelected(false)
	} else {
		s.Cursor.Focus(nil)
	}
}

func (s *seq) unopen(it ListItem) {
	for i, o := range s.opening {
		if o == it {
			s.opening = append(s.opening[:i], s.opening[i+1:]...)
			return
		}
	}
}

// placeNew finishes the layout of items new to the list, once their targets
// are set: those put in with SetItems appear in place, those inserted on
// screen start closed and open up around their middle.
func (s *seq) placeNew() {
	if s.placeAll {
		for _, it := range s.items {
			it.Snap()
		}
		s.placeAll = false
	}
	for _, it := range s.opening {
		it.Snap()
		f := it.Frame()
		if s.vertical {
			f.Y.Current += f.H.Target / 2
			f.H.Current = 0
		} else {
			f.X.Current += f.W.Target / 2
			f.W.Current = 0
		}
	}
	clear(s.opening)
	s.opening = s.opening[:0]
}

// move is the press and release of a key that moves the selection by dir
// along the axis. At an edge in Pass it is left to the container, at any
// other edge the content is pulled. It reports whether the key is the
// list's.
func (s *seq) move(e Event, dir int) bool {
	s.settle()
	n := s.next(s.selected, dir)
	edge := EdgeEnd
	if dir < 0 {
		edge = EdgeStart
	}
	if e.Phase == Press && n < 0 && s.Pass&edge != 0 {
		return false
	}
	var dx, dy, pdx, pdy float32
	if s.vertical {
		dy = float32(dir)
		// The over-drag at an end is pulled the opposite way on the
		// vertical axis, to match the sideways one's feel on a stick:
		// holding towards an edge pulls the content away from it, not
		// further past it.
		pdy = -dy
	} else {
		dx, pdx = float32(dir), float32(dir)
	}
	if push(s.Cursor, &s.pull, e, n < 0, dx, dy, pdx, pdy) && n >= 0 {
		s.Select(n)
	}
	return true
}

// enter is the press and release of enter on an item that did not take it:
// OnSelect, if there is one.
func (s *seq) enter(e Event) bool {
	if s.OnSelect == nil || s.SelectedItem() == nil {
		return false
	}
	if click(s.Cursor, e) && s.selected >= 0 {
		s.OnSelect(s.selected)
	}
	return true
}

// trail pushes the items off their slots by dx times their cascade
// distance, so they come in behind the selection. Items that cascade
// themselves pass it on to their own items.
func (s *seq) trail(dx float32) {
	for i, it := range s.items {
		m := dx * s.spread(i)
		if c, ok := it.(cascader); ok {
			c.trail(m)
		} else {
			it.Frame().X.Current += m * s.Cascade
		}
	}
}

// lag kicks the items by dx times their cascade distance, so they fall
// behind the selection as the list flies away.
func (s *seq) lag(dx float32) {
	for i, it := range s.items {
		m := dx * s.spread(i)
		if c, ok := it.(cascader); ok {
			c.lag(m)
		} else {
			it.Frame().X.Kick(m * s.Cascade)
		}
	}
}

// spring sets the springs of an item rel places away from the selection:
// slower the further away, with Bounce along the axis.
func (s *seq) spring(f *AnimatedRect, rel int) {
	w := stagger(s.ItemSpeed, s.Stagger, rel)
	f.X.Spring(w, 1)
	f.Y.Spring(w, 1)
	f.W.Spring(w, 1)
	f.H.Spring(w, 1)
	if s.vertical {
		f.Y.Spring(w, s.Bounce)
	} else {
		f.X.Spring(w, s.Bounce)
	}
}

func (s *seq) spread(i int) float32 {
	return float32(1 + absInt(i-max(s.selected, 0)))
}

// update updates the items and the removed ones still closing up, and
// releases those that have closed.
func (s *seq) update(dt float32) {
	for _, it := range s.items {
		it.Update(dt)
	}
	n := 0
	for _, it := range s.leaving {
		if it.Frame().Settled() {
			it.Release()
			continue
		}
		it.Update(dt)
		s.leaving[n] = it
		n++
	}
	clear(s.leaving[n:])
	s.leaving = s.leaving[:n]
}

// draw draws the items that show in c, removed ones still closing up
// included, offset by the pull.
func (s *seq) draw(c Canvas) {
	clip := c.Clip()
	var dx, dy int16
	if s.vertical {
		dy = s.pull.Int()
	} else {
		dx = s.pull.Int()
	}
	ic := c.Translate(dx, dy)
	for _, it := range s.leaving {
		drawIn(ic, it, clip, dx, dy)
	}
	for _, it := range s.items {
		drawIn(ic, it, clip, dx, dy)
	}
}

// drawIn draws it if, moved by dx, dy, it overlaps clip.
func drawIn(c Canvas, it ListItem, clip Rect, dx, dy int16) {
	r := it.Frame().Rect()
	r.X += dx
	r.Y += dy
	if r.Empty() || r.Intersect(clip).Empty() {
		return
	}
	it.Draw(c)
}

func (s *seq) releaseLeaving() {
	for _, it := range s.leaving {
		it.Release()
	}
	clear(s.leaving)
	s.leaving = s.leaving[:0]
}

// release unregisters the animated values of the items, removed ones
// included.
func (s *seq) release() {
	s.pull.Release()
	for _, it := range s.items {
		it.Release()
	}
	s.releaseLeaving()
}

// snap finishes the animations of the items. Removed items vanish.
func (s *seq) snap() {
	s.pull.Snap()
	for _, it := range s.items {
		it.Snap()
	}
	s.releaseLeaving()
}
