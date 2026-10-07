package ui

import (
	"osu-mc/internal/animation"
)

// caretBlinkPeriod is how long the caret stays on or off. Redraws only
// happen on the toggle, not every frame, so a blinking caret costs nothing
// while nobody is looking at it.
const caretBlinkPeriod = 0.5

// typeKick is how far the text field bounces vertically when a character
// lands. A key that has no effect shoves it sideways by edgeKick instead.
const typeKick = 2

// edgeKick is how far the field is shoved when a key has no effect.
const edgeKick = 4

// keyKind says what a key on the keyboard does when it is selected.
type keyKind uint8

const (
	keyChar      keyKind = iota // types a rune (r), upper-cased while caps is on
	keyBackspace                // deletes the last rune
	keyCaps                     // toggles caps lock and relabels the letters
	keyLayer                    // switches to another layer (layer)
	keyDone                     // calls OnDone
)

// keyDef describes one key. label is only used by keyLayer; every other
// kind computes its label from state (case, or a fixed word), see
// Keyboard.label.
type keyDef struct {
	kind  keyKind
	r     rune
	label string
	layer int
}

func char(r rune) keyDef { return keyDef{kind: keyChar, r: r} }
func toLayer(label string, l int) keyDef {
	return keyDef{kind: keyLayer, label: label, layer: l}
}

// The three layers, QWERTY-ish so the common letters are the quickest to
// reach. Every row is a Row in its own right and scrolls to reach keys past
// the edge of the screen, so a row does not need to fit on screen at once.
//
// Letters carries its own case (see Keyboard.caps) instead of being a
// fourth layer, so shifting case is a single key rather than a detour
// through another page. Digits and symbols between them cover the rest of
// 0x21..0x7E; see TestKeyboardCoversAllChars for the proof.
const (
	layerLetters = iota
	layerDigits
	layerSymbols
)

var keysLetters = [3][]keyDef{
	{char('q'), char('w'), char('e'), char('r'), char('t'), char('y'), char('u'), char('i'), char('o'), char('p')},
	{char('a'), char('s'), char('d'), char('f'), char('g'), char('h'), char('j'), char('k'), char('l')},
	{
		{kind: keyCaps},
		char('z'), char('x'), char('c'), char('v'), char('b'), char('n'), char('m'),
		{kind: keyBackspace},
		char(' '),
		toLayer("123", layerDigits),
		{kind: keyDone},
	},
}

var keysDigits = [3][]keyDef{
	{char('1'), char('2'), char('3'), char('4'), char('5'), char('6'), char('7'), char('8'), char('9'), char('0')},
	{char(','), char('.'), char('?'), char('!'), char('\''), char('-'), char(':'), char(';'), char('/')},
	{
		toLayer("ABC", layerLetters), toLayer("#+=", layerSymbols),
		char('@'), char('&'), char('('), char(')'),
		{kind: keyBackspace}, char(' '), {kind: keyDone},
	},
}

var keysSymbols = [3][]keyDef{
	{char('"'), char('#'), char('$'), char('%'), char('*'), char('+'), char('<'), char('='), char('>'), char('[')},
	{char('\\'), char(']'), char('^'), char('_'), char('`'), char('{'), char('|'), char('}'), char('~')},
	{
		toLayer("123", layerDigits), toLayer("ABC", layerLetters),
		{kind: keyBackspace}, char(' '), {kind: keyDone},
	},
}

func layerTable(idx int) [3][]keyDef {
	switch idx {
	case layerDigits:
		return keysDigits
	case layerSymbols:
		return keysSymbols
	default:
		return keysLetters
	}
}

// Keyboard is a full screen page that types any visible ASCII character
// (0x20..0x7E) with only the d-pad and enter.
//
// It is a List of a text field and three key Rows, built entirely from
// those types: Keyboard neither reimplements scrolling or springs nor
// changes Row or List, it just drives them. The field is a Static Text, a
// line the selection passes over, laid out and animated by the List exactly
// as any other line would be, which is also why the field can be "kicked"
// like a spring on every keystroke (see typeRune) - List.Layout puts every
// item, field included, into spring motion.
//
// The focus path is keyboard → list → row → key. Keyboard implements
// Interceptor to claim KeyEnter in the capture phase, before it would
// otherwise bubble up from whichever row is focused. That
// lets one press/release handler (see InterceptEvent) serve all three key
// rows instead of wiring an OnSelect closure into each, and demonstrates
// the capture path described in event.go.
//
// A Keyboard registers animated values through its field and rows; create
// one with NewKeyboard and do not copy it.
type Keyboard struct {
	AnimatedRect

	// Cursor gets the press/release squeeze on Enter and follows the
	// selected key; defaults to DefaultCursor.
	Cursor *Cursor

	// MaxLen caps the number of characters that can be typed; 0 means
	// unlimited.
	MaxLen int

	// Placeholder is shown in brackets in the field while nothing is
	// typed. Set it before SetText.
	Placeholder string

	// OnDone is called with the typed text when the OK key is released.
	OnDone func(s string)
	// OnChange, if set, is called with the typed text after every edit
	// (typing, backspace or SetText).
	OnChange func(s string)

	lines *List
	field *Text
	rows  [3]*Row

	specs [3][]keyDef
	layer int
	caps  bool

	buf []byte

	caretOn bool
	caretT  float32
}

// NewKeyboard returns a keyboard on the letters layer with an empty text
// field. Its size comes from SetTarget, like a List.
func NewKeyboard() *Keyboard {
	k := &Keyboard{Cursor: DefaultCursor}
	k.InitRect(RectF{}, SpeedLayout)

	k.field = NewText("", nil)
	k.field.Static = true

	for i := range k.rows {
		k.rows[i] = NewRow()
		// A stray left at the start of a row must not abandon typing;
		// leaving is KeyBack or the OK key only.
		k.rows[i].Pass = 0
	}

	k.lines = NewList(k.field, k.rows[0], k.rows[1], k.rows[2])
	k.lines.ItemHeight = RowHeight
	k.lines.Gap = 0
	k.lines.ScrollMargin = 0
	k.lines.Marquee = false
	k.shareCursor()

	k.setLayer(layerLetters, false)
	return k
}

// Text returns the typed string.
func (k *Keyboard) Text() string { return string(k.buf) }

// SetText replaces the typed string, clamped to MaxLen if one is set.
func (k *Keyboard) SetText(s string) {
	if k.MaxLen > 0 && len(s) > k.MaxLen {
		s = s[:k.MaxLen]
	}
	k.buf = append(k.buf[:0], s...)
	k.syncField()
}

func (k *Keyboard) syncField() {
	if len(k.buf) == 0 && k.Placeholder != "" {
		k.field.SetText(bracket(k.Placeholder))
	} else {
		k.field.SetText(string(k.buf))
	}
	k.field.ScrollTo(k.field.Overflow())
	// Keep the caret solid while typing, like any text box; it starts
	// blinking again once the typing stops.
	k.caretOn, k.caretT = true, 0
	animation.Default.Invalidate()
	if k.OnChange != nil {
		k.OnChange(k.Text())
	}
}

// upperIf upper-cases r while on is true and r is a lowercase ASCII letter.
func upperIf(on bool, r rune) rune {
	if on && r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}

// label returns what a key currently displays. Most kinds have a fixed
// word; a char key shows its rune (upper-cased while caps is on) except
// for space, which gets a readable name since the font has nothing to show
// for 0x20 itself.
func (k *Keyboard) label(d keyDef) string {
	switch d.kind {
	case keyChar:
		if d.r == ' ' {
			return "SPC"
		}
		return string(upperIf(k.caps, d.r))
	case keyBackspace:
		return "DEL"
	case keyCaps:
		if k.caps {
			return "CAPS"
		}
		return "caps"
	case keyDone:
		return "OK"
	default: // keyLayer
		return d.label
	}
}

// setLayer switches the keys shown in all three rows. Labels are rebuilt
// from the layer's table into fresh Text items, which is simpler than
// keeping every layer's items alive at once; that is a modest allocation
// for a user action (switching layers), not a per-frame one. animate
// cascades the new keys in from the direction of travel, the same trick
// Row and List use when a page flies in; it is skipped for the initial
// layer at construction, when there is nothing to animate from.
func (k *Keyboard) setLayer(idx int, animate bool) {
	k.layer = idx
	defs := layerTable(idx)
	k.specs = defs

	// SetItems keeps roughly the same place in the row, so a layer key
	// pressed twice in a row does not send the cursor back to the start,
	// and moves the cursor onto the new keys if it was on an old one.
	for i, row := range k.rows {
		specs := defs[i]
		items := make([]ListItem, len(specs))
		for j, d := range specs {
			items[j] = NewText(k.label(d), nil)
		}
		row.SetItems(items...)
		row.Layout()
		if animate {
			row.trail(1)
		}
	}
}

// refreshCaseLabels relabels the letters layer's char and caps keys after
// caps is toggled, in place, without rebuilding the rows: exactly the
// "labels change via SetText" animation the layout calls for, since Text's
// own SetText already invalidates the frame and Row's item widths already
// react to new label widths on the next Layout.
func (k *Keyboard) refreshCaseLabels() {
	for i, row := range k.rows {
		for j, d := range k.specs[i] {
			if d.kind == keyChar || d.kind == keyCaps {
				row.Item(j).(*Text).SetText(k.label(d))
			}
		}
	}
}

// InterceptEvent claims KeyEnter in the capture phase for every key row at
// once: see the Keyboard doc comment for why. It replicates click()'s
// press/release feedback itself, since claiming the event here means the
// row that would otherwise do that never sees it.
func (k *Keyboard) InterceptEvent(e Event) bool {
	if e.Key != KeyEnter {
		return false
	}
	if !click(k.Cursor, e) {
		return true // press: feedback only, claimed regardless
	}
	k.activateFocused()
	return true
}

func (k *Keyboard) activateFocused() {
	el := k.lines.Focused()
	for i, row := range k.rows {
		if Element(row) == el {
			k.activate(i, row.Selected())
			return
		}
	}
}

func (k *Keyboard) activate(rowIdx, itemIdx int) {
	specs := k.specs[rowIdx]
	if itemIdx < 0 || itemIdx >= len(specs) {
		return
	}
	switch d := specs[itemIdx]; d.kind {
	case keyChar:
		k.typeRune(d.r)
	case keyBackspace:
		k.backspace()
	case keyCaps:
		k.caps = !k.caps
		k.refreshCaseLabels()
	case keyLayer:
		k.setLayer(d.layer, true)
	case keyDone:
		if k.OnDone != nil {
			k.OnDone(k.Text())
		}
	}
}

// typeRune appends r (upper-cased while caps is on) to the text, unless
// MaxLen is already reached. The field kicks to acknowledge the character
// and the cursor gets an extra bump, on top of the press/release squeeze
// every key already gets from InterceptEvent.
func (k *Keyboard) typeRune(r rune) {
	r = upperIf(k.caps, r)
	if k.MaxLen > 0 && len(k.buf) >= k.MaxLen {
		k.field.X.Kick(edgeKick)
		return
	}
	k.buf = append(k.buf, byte(r))
	k.syncField()
	k.field.Y.Kick(-typeKick)
	if k.Cursor != nil {
		k.Cursor.Bump(1)
	}
}

func (k *Keyboard) backspace() {
	if len(k.buf) == 0 {
		k.field.X.Kick(edgeKick)
		return
	}
	k.buf = k.buf[:len(k.buf)-1]
	k.syncField()
	k.field.Y.Kick(typeKick)
}

// Focused returns the list, so the Router's path runs keyboard → list →
// selected row: left and right go to the row, up and down bubble to the
// list, enter is intercepted here before either sees it.
func (k *Keyboard) Focused() Element { return k.lines }

// shareCursor hands Cursor down to the list and rows, which drive it, so
// that setting Keyboard.Cursor after NewKeyboard takes effect.
func (k *Keyboard) shareCursor() {
	k.lines.Cursor = k.Cursor
	for _, r := range k.rows {
		r.Cursor = k.Cursor
	}
}

// Focus puts the cursor on the focused row's selected key.
func (k *Keyboard) Focus() {
	k.shareCursor()
	k.lines.Focus()
}

// OnEnter is called by a Viewer when the keyboard flies in.
func (k *Keyboard) OnEnter(resume bool) {
	k.shareCursor()
	k.lines.OnEnter(resume)
}

// OnLeave is called by a Viewer when the keyboard flies out.
func (k *Keyboard) OnLeave(exit bool) { k.lines.OnLeave(exit) }

// Release unregisters every animated value of the keyboard, its field and
// its rows. A Viewer calls it once the keyboard has flown out.
func (k *Keyboard) Release() {
	k.lines.Release()
	k.AnimatedRect.Release()
}

// Snap lays the keyboard out and finishes all running animations.
func (k *Keyboard) Snap() {
	k.AnimatedRect.Snap()
	k.lines.Snap()
}

// Update lays out the list from the keyboard's own target rect and
// advances the caret blink. It allocates nothing: the only allocations a
// Keyboard makes are on setLayer and typing, both user actions.
func (k *Keyboard) Update(dt float32) {
	k.lines.SetTarget(k.Target())
	k.lines.Update(dt)

	k.caretT += dt
	if k.caretT < caretBlinkPeriod {
		return
	}
	k.caretT -= caretBlinkPeriod
	k.caretOn = !k.caretOn
	animation.Default.Invalidate()
}

// Draw draws the list and, while it is on, a caret after the visible
// text: the field is always scrolled so the tail of an overflowing string
// stays visible (see syncField), so the caret sits at the end of whatever
// is showing.
func (k *Keyboard) Draw(c Canvas) {
	k.lines.Draw(c)
	if !k.caretOn {
		return
	}
	fr := k.field.Rect()
	if fr.H <= 2 {
		return
	}
	// With nothing typed the caret sits at the start, before the
	// placeholder.
	x := fr.X
	if len(k.buf) > 0 {
		x += int16(k.field.ContentWidth()) - animation.Round(k.field.Scroll())
	}
	x = min(x, fr.X+fr.W-1)
	c.FillRect(x, fr.Y+1, 1, fr.H-2)
}
