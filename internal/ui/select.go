package ui

// Select is a line showing one of a set of options, that opens a page to
// change it: enter or right pushes a List of the options onto Viewer with the
// current one selected, and choosing one writes it back, calls OnChange and
// pops the page. Going back from the page leaves the value as it was.
//
// It is a ListItem: it goes in a List like any other line, the way a settings
// page is a list of choices. The list places the select's frame; the text
// sits inside it, Pad from either side, and scrolls back and forth while the
// cursor is on it if it does not fit.
//
// Selects register animated values, create them with NewSelect and do not
// copy them.
type Select struct {
	AnimatedRect

	// Pad is the space left and right of the text inside the frame.
	Pad float32

	// Label heads the page of options, and Format puts it in front of the
	// value in the line itself.
	Label string

	// Format, if set, turns the chosen option into what the line shows, for
	// example to put Label in front of it. Without one the line is the
	// option on its own.
	Format func(option string) string

	// Empty is shown in brackets while there is nothing to choose from.
	Empty string

	// Viewer is where the page of options is pushed; without one the value
	// cannot be changed.
	Viewer *Viewer

	// Cursor follows the select while it is focused, and the options while
	// the page is open.
	Cursor *Cursor

	// OnChange is called with the chosen index when an option is released,
	// after the value has been set and the page popped. Setting the index in
	// code does not call it.
	OnChange func(i int)

	text    *Text
	options []string
	index   int
}

// NewSelect returns a select on options with the first one chosen, or showing
// empty in brackets if there are none. It has no Pad, so in a List its text
// lines up with the texts around it.
func NewSelect(v *Viewer, label string, options ...string) *Select {
	s := &Select{
		Label:   label,
		Empty:   label,
		Viewer:  v,
		Cursor:  DefaultCursor,
		text:    NewText("", nil),
		options: append([]string(nil), options...),
	}
	s.InitRect(RectF{H: RowHeight}, SpeedLayout)
	s.place()
	s.sync()
	return s
}

// Len returns the number of options.
func (s *Select) Len() int { return len(s.options) }

// Option returns option i.
func (s *Select) Option(i int) string { return s.options[i] }

// Index returns the index of the chosen option, or -1 while there are none.
func (s *Select) Index() int {
	if len(s.options) == 0 {
		return -1
	}
	return s.index
}

// Value returns the chosen option, or the empty string while there are none.
func (s *Select) Value() string {
	if len(s.options) == 0 {
		return ""
	}
	return s.options[s.index]
}

// SetIndex chooses option i, clamped to the options there are. It does not
// call OnChange.
func (s *Select) SetIndex(i int) {
	if len(s.options) == 0 {
		return
	}
	i = min(max(i, 0), len(s.options)-1)
	if i == s.index {
		return
	}
	s.index = i
	s.sync()
}

// SetOptions replaces the options. The chosen index is kept as far as the new
// options go.
func (s *Select) SetOptions(options ...string) {
	s.options = append(s.options[:0:0], options...)
	s.index = min(max(s.index, 0), max(0, len(s.options)-1))
	s.sync()
}

// Field returns the Text that draws the line, to change its font or
// alignment.
func (s *Select) Field() *Text { return s.text }

// sync shows the chosen option, from its start: a marquee that was half way
// along the old text starts over.
func (s *Select) sync() {
	s.text.SetMarquee(false)
	s.text.ScrollTo(0)
	switch {
	case len(s.options) == 0:
		s.text.SetText(bracket(s.Empty))
	case s.Format != nil:
		s.text.SetText(s.Format(s.options[s.index]))
	default:
		s.text.SetText(s.options[s.index])
	}
}

// Open pushes a page of the options onto Viewer, with the chosen one
// selected. Choosing writes it back and pops the page; going back leaves the
// value alone.
func (s *Select) Open() {
	v := s.Viewer
	if v == nil || len(s.options) == 0 {
		return
	}

	// The page is built fresh every time: a Viewer releases a page once it
	// has flown out, so the old one must not be pushed again.
	page := NewList()
	page.Cursor = s.Cursor
	page.SetTarget(RectF{W: v.Width, H: v.Height})

	// The heading is not selectable, so the selection passes over it and the
	// options start one line below.
	head := 0
	if s.Label != "" {
		title := NewText(s.Label, nil)
		title.Static = true
		page.Add(title)
		head = 1
	}
	for _, o := range s.options {
		page.Add(NewText(o, nil))
	}
	page.Select(head + s.index)
	page.OnSelect = func(i int) {
		i -= head
		if i < 0 || i >= len(s.options) {
			return
		}
		s.SetIndex(i)
		if v.Top() == Element(page) {
			v.Pop()
		}
		if s.OnChange != nil {
			s.OnChange(s.index)
		}
	}
	v.Push(page)
}

// Focus puts the cursor on the select.
func (s *Select) Focus() {
	if s.Cursor != nil {
		s.Cursor.Focus(s)
	}
}

// Size is as wide as the text and Pad, and as high as the select is made.
func (s *Select) Size() (w, h float32) {
	return float32(s.text.ContentWidth()) + 2*s.Pad, s.H.Target
}

// Selectable reports true: a select is there to be opened.
func (s *Select) Selectable() bool { return true }

func (s *Select) setLiveFocus(a Axis) { s.text.LiveFocus = a }

// HandleEvent opens the page of options when enter or right is released, with
// the same press feedback as a List: enter squeezes the cursor, right leans
// it.
func (s *Select) HandleEvent(e Event) bool {
	switch e.Key {
	case KeyEnter:
		if click(s.Cursor, e) {
			s.Open()
		}
	case KeyRight:
		step(s.Cursor, e, 1, 0)
		if e.Phase == Release {
			s.Open()
		}
	default:
		return false
	}
	return true
}

// FocusRect is the text's, so the cursor covers what is shown.
func (s *Select) FocusRect() RectF { return s.text.FocusRect() }

// Release unregisters the select's animated values.
func (s *Select) Release() {
	s.AnimatedRect.Release()
	s.text.Release()
}

// Snap finishes the select's animations, the text's included.
func (s *Select) Snap() {
	s.AnimatedRect.Snap()
	s.place()
}

// place puts the text inside the frame, Pad in from either side. The text is
// carried along with the frame rather than springing after it, so the two
// never drift apart.
func (s *Select) place() {
	t, c := s.Target(), s.Current()
	s.text.SetTarget(RectF{t.X + s.Pad, t.Y, max(0, t.W-2*s.Pad), t.H})
	s.text.X.Current = c.X + s.Pad
	s.text.Y.Current = c.Y
	s.text.W.Current = max(0, c.W-2*s.Pad)
	s.text.H.Current = c.H
}

func (s *Select) Update(dt float32) {
	s.place()
	// Like the selection of a List, the text scrolls while the cursor is on
	// it and goes back to its start when the cursor leaves.
	focused := s.Cursor != nil && s.Cursor.Target() == Focusable(s)
	if !focused && s.text.Marquee() {
		s.text.ScrollTo(0)
	}
	s.text.SetMarquee(focused)
	s.text.Update(dt)
}

func (s *Select) Draw(c Canvas) { s.text.Draw(c) }
