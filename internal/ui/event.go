package ui

// Key is a logical input key.
type Key uint8

const (
	KeyUp Key = iota + 1
	KeyDown
	KeyLeft
	KeyRight
	KeyEnter
	KeyBack
	keyCount
)

// Phase is what happened to a key.
type Phase uint8

const (
	Press   Phase = iota + 1 // went down
	Repeat                   // still held, auto repeat
	Release                  // let go
)

// Event is a key event.
//
// Keys go down and come up as separate events so the UI can answer both: the
// press starts the feedback (the cursor squeezes or leans, the page peeks)
// and the release commits the action. A held key may also repeat; the
// release that ends a run of repeats has Repeated set, so an action that
// already happened on the repeats is not done once more.
type Event struct {
	Key      Key
	Phase    Phase
	Repeated bool
}

// Fires reports whether e triggers the action of a key that acts on release
// and on every repeat: a Repeat, or a Release that did not end repeats.
func (e Event) Fires() bool {
	return e.Phase == Repeat || e.Phase == Release && !e.Repeated
}

// Handler is an element that takes key events in the bubbling phase: the
// focused element is asked first, then its containers outwards.
//
// Returning true from a Press claims the key: its Repeats and its Release
// then go straight to this handler, wherever the focus has moved in the
// meantime. Returning false passes the press on to the container.
type Handler interface {
	HandleEvent(e Event) bool
}

// Interceptor is a container that sees presses before its focused
// descendants do, in the capture phase, from the root inwards. Returning true
// claims the key like Handler does, and the descendants never see it.
type Interceptor interface {
	InterceptEvent(e Event) bool
}

// Container is an element with a focused child that events are routed to.
// Focused must return a nil interface, not a typed nil, when nothing has the
// focus.
type Container interface {
	Focused() Element
}

// Focuser is an element that can take the focus. Focus moves the cursor
// onto it, or onto its selection.
type Focuser interface {
	Focus()
}

// maxDepth bounds the focus path, so routing does not allocate.
const maxDepth = 16

// owner is whoever claimed a key, and in which phase.
type owner struct {
	el      Element
	capture bool
}

func (o owner) deliver(e Event) bool {
	if o.capture {
		return o.el.(Interceptor).InterceptEvent(e)
	}
	return o.el.(Handler).HandleEvent(e)
}

// Router delivers key events along the focus path: Root, then whatever it
// reports as Focused, and so on down to an element that is not a Container.
//
// A Press goes through the capture phase, root to leaf, then the bubbling
// phase, leaf to root. The first element to return true owns the key until
// it is released: its Repeats and Release go to it alone. That keeps press
// and release paired even when the press moves the focus, and makes sure a
// release never lands on something that did not see the press. A press that
// nobody claims is dropped, together with its repeats and release.
type Router struct {
	Root Element

	owners [keyCount]owner
}

// Dispatch routes e and reports whether anything took it.
func (r *Router) Dispatch(e Event) bool {
	if e.Key == 0 || e.Key >= keyCount {
		return false
	}
	if e.Phase != Press {
		o := r.owners[e.Key]
		if e.Phase == Release {
			r.owners[e.Key] = owner{}
		}
		if o.el == nil {
			return false
		}
		o.deliver(e)
		return true
	}

	r.owners[e.Key] = owner{}
	var buf [maxDepth]Element
	path := buf[:0]
	for el := r.Root; el != nil && len(path) < maxDepth; {
		path = append(path, el)
		c, ok := el.(Container)
		if !ok {
			break
		}
		el = c.Focused()
	}

	for _, el := range path {
		if i, ok := el.(Interceptor); ok && i.InterceptEvent(e) {
			r.owners[e.Key] = owner{el, true}
			return true
		}
	}
	for i := len(path) - 1; i >= 0; i-- {
		if h, ok := path[i].(Handler); ok && h.HandleEvent(e) {
			r.owners[e.Key] = owner{path[i], false}
			return true
		}
	}
	return false
}

// Cancel forgets every claimed key. The Release of a key that was down is
// then dropped.
func (r *Router) Cancel() {
	r.owners = [keyCount]owner{}
}
