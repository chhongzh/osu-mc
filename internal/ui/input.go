package ui

// Input is a one-line text field. It is a Text that shows the value, or the
// placeholder in brackets while the value is empty, and that opens a
// Keyboard to edit it: enter or right pushes the keyboard onto Viewer, the
// keyboard's OK key writes the text back, calls OnDone and pops the
// keyboard. Going back from the keyboard cancels and leaves the value as it
// was.
//
// It is a ListItem: it goes in a List like any other line, for example as
// the first line of a list it searches. The list places the input's frame;
// the text sits inside it, Pad from either side, and scrolls back and forth
// while the cursor is on it if it does not fit.
//
// Inputs register animated values, create them with NewInput and do not
// copy them.
type Input struct {
	AnimatedRect

	// Pad is the space left and right of the text inside the frame.
	Pad float32

	// Placeholder is shown in brackets while the value is empty, both in
	// the input and in its keyboard.
	Placeholder string

	// MaxLen caps the length of the value; 0 means unlimited.
	MaxLen int

	// Format, if set, turns a value that is not empty into what the input
	// shows, for example to put a label in front of it.
	Format func(value string) string

	// Viewer is where the keyboard is pushed; without one the input
	// cannot be edited.
	Viewer *Viewer

	// Cursor follows the input while it is focused, and the keyboard's
	// keys while it is open.
	Cursor *Cursor

	// OnDone is called with the new value when the keyboard's OK key is
	// released, after the value has been set and the keyboard popped.
	OnDone func(s string)

	text  *Text
	value string
}

// NewInput returns an empty input, one row high, showing placeholder in
// brackets. It has no Pad, so in a List its text lines up with the texts
// around it.
func NewInput(v *Viewer, placeholder string) *Input {
	in := &Input{
		Placeholder: placeholder,
		Viewer:      v,
		Cursor:      DefaultCursor,
		text:        NewText("", nil),
	}
	in.InitRect(RectF{H: RowHeight}, SpeedLayout)
	in.place()
	in.sync()
	return in
}

// Text returns the value.
func (in *Input) Text() string { return in.value }

// SetText sets the value, cut to MaxLen if one is set.
func (in *Input) SetText(s string) {
	if in.MaxLen > 0 && len(s) > in.MaxLen {
		s = s[:in.MaxLen]
	}
	in.value = s
	in.sync()
}

// Field returns the Text that draws the input, to change its font or
// alignment.
func (in *Input) Field() *Text { return in.text }

// sync shows the value, from its start: a marquee that was half way along
// the old text starts over.
func (in *Input) sync() {
	in.text.SetMarquee(false)
	in.text.ScrollTo(0)
	switch {
	case in.value == "":
		in.text.SetText(bracket(in.Placeholder))
	case in.Format != nil:
		in.text.SetText(in.Format(in.value))
	default:
		in.text.SetText(in.value)
	}
}

func bracket(s string) string { return "[" + s + "]" }

// Open pushes a keyboard onto Viewer to edit the value.
func (in *Input) Open() {
	v := in.Viewer
	if v == nil {
		return
	}
	k := NewKeyboard()
	k.Cursor = in.Cursor
	k.MaxLen = in.MaxLen
	k.Placeholder = in.Placeholder
	k.SetTarget(RectF{W: v.Width, H: v.Height})
	k.SetText(in.value)
	k.OnDone = func(s string) {
		in.SetText(s)
		if v.Top() == Element(k) {
			v.Pop()
		}
		if in.OnDone != nil {
			in.OnDone(in.value)
		}
	}
	v.Push(k)
}

// Focus puts the cursor on the input.
func (in *Input) Focus() {
	if in.Cursor != nil {
		in.Cursor.Focus(in)
	}
}

// Size is as wide as the text and Pad, and as high as the input is made.
func (in *Input) Size() (w, h float32) {
	return float32(in.text.ContentWidth()) + 2*in.Pad, in.H.Target
}

// Selectable reports true: an input is there to be edited.
func (in *Input) Selectable() bool { return true }

func (in *Input) setLiveFocus(a Axis) { in.text.LiveFocus = a }

// HandleEvent opens the keyboard when enter or right is released, with the
// same press feedback as a List: enter squeezes the cursor, right leans it.
func (in *Input) HandleEvent(e Event) bool {
	switch e.Key {
	case KeyEnter:
		if click(in.Cursor, e) {
			in.Open()
		}
	case KeyRight:
		step(in.Cursor, e, 1, 0)
		if e.Phase == Release {
			in.Open()
		}
	default:
		return false
	}
	return true
}

// FocusRect is the text's, so the cursor covers what is shown.
func (in *Input) FocusRect() RectF { return in.text.FocusRect() }

// Release unregisters the input's animated values.
func (in *Input) Release() {
	in.AnimatedRect.Release()
	in.text.Release()
}

// Snap finishes the input's animations, the text's included.
func (in *Input) Snap() {
	in.AnimatedRect.Snap()
	in.place()
}

// place puts the text inside the frame, Pad in from either side. The text is
// carried along with the frame rather than springing after it, so the two
// never drift apart.
func (in *Input) place() {
	t, c := in.Target(), in.Current()
	in.text.SetTarget(RectF{t.X + in.Pad, t.Y, max(0, t.W-2*in.Pad), t.H})
	in.text.X.Current = c.X + in.Pad
	in.text.Y.Current = c.Y
	in.text.W.Current = max(0, c.W-2*in.Pad)
	in.text.H.Current = c.H
}

func (in *Input) Update(dt float32) {
	in.place()
	// Like the selection of a List, the text scrolls while the cursor is
	// on it and goes back to its start when the cursor leaves.
	focused := in.Cursor != nil && in.Cursor.Target() == Focusable(in)
	if !focused && in.text.Marquee() {
		in.text.ScrollTo(0)
	}
	in.text.SetMarquee(focused)
	in.text.Update(dt)
}

func (in *Input) Draw(c Canvas) { in.text.Draw(c) }
