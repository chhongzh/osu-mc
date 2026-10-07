// Package application holds the apps of osu!mc: the launcher, the settings,
// the Lorem Ipsum demo, the HTTP client and the system info. Every page is
// a ui.List or a ui.TextView pushed onto one ui.Viewer.
//
// The hardware is reached through the interfaces in services.go, so the
// apps build and are tested on the host; cmd/osu-mc provides the real ones.
package application

import (
	"sync"

	"osu-mc/internal/ui"
)

// App is the set of apps sharing one viewer and the services.
type App struct {
	Viewer *ui.Viewer
	Services

	// Shortcuts are the saved requests of the HTTP client. They outlive
	// its pages, which are built afresh every time they are opened.
	Shortcuts Shortcuts

	// passwords are the Wi-Fi passwords typed since boot, by SSID.
	passwords map[string]string

	// The HTTP client's editor, kept while its page is closed: the URL and
	// the shortcut it was loaded from, which Save then updates.
	url    string
	linked *Shortcut
}

// New returns the apps for v. Call Start to show the launcher.
func New(v *ui.Viewer, s Services) *App {
	return &App{Viewer: v, Services: s,
		Shortcuts: Shortcuts{list: []*Shortcut{
			{URL: "http://httpbin.org/get"},
		}}}
}

// Start pushes the launcher, the root page that is never popped.
func (a *App) Start() {
	a.Viewer.Push(a.newHome())
}

// newHome returns the launcher: one line per app.
func (a *App) newHome() *ui.List {
	title := ui.NewText("osu!mc", nil)
	title.Static = true
	title.Center = true

	settings := ui.NewText("Settings", nil)
	lorem := ui.NewText("Lorem Ipsum", nil)
	client := ui.NewText("HTTP Client", nil)
	system := ui.NewText("System Info", nil)

	l := a.newList(title, settings, lorem, client, system)
	l.OnSelect = func(int) {
		switch l.SelectedItem() {
		case settings:
			a.Viewer.Push(a.newSettings())
		case lorem:
			a.Viewer.Push(a.newLorem())
		case client:
			a.Viewer.Push(a.newHTTP())
		case system:
			a.Viewer.Push(a.newSystem())
		}
	}
	return l
}

// newList returns a full screen page of items, one row each. Right is the
// same as enter; left is not the list's, so it bubbles up to the viewer and
// goes back.
func (a *App) newList(items ...ui.ListItem) *ui.List {
	l := ui.NewList(items...)
	l.ItemHeight = ui.RowHeight
	// Every page indents the same way, so moving between apps does not shift
	// the text sideways, and the cursor's margin is not cut off by the left
	// edge of the screen.
	l.Offset = ui.Stagger(4, 5)
	l.SetTarget(a.page())
	l.OnRight = func() {
		if l.OnSelect != nil {
			l.OnSelect(l.Selected())
		}
	}
	return l
}

// newTextView returns a full screen, wrapped view of s.
func (a *App) newTextView(s string) *ui.TextView {
	v := ui.NewTextView(s, nil, true)
	v.SetTarget(a.page())
	return v
}

func (a *App) page() ui.RectF {
	return ui.RectF{W: a.Viewer.Width, H: a.Viewer.Height}
}

// newLabels returns a text for each label.
func newLabels(labels ...string) []ui.ListItem {
	items := make([]ui.ListItem, len(labels))
	for i, s := range labels {
		items[i] = ui.NewText(s, nil)
	}
	return items
}

// spawn runs a blocking call off the UI loop. Tests replace it to run the
// call in place, so that its result is there on the next Update.
var spawn = func(f func()) { go f() }

// job is the result of one blocking call that runs on its own goroutine,
// like a scan or a request. The UI starts it on a key release and polls it
// in Update, so the frame never waits and all UI state stays on the UI
// goroutine: the call only fills in the result, under the lock.
type job[T any] struct {
	mu      sync.Mutex
	running bool
	done    bool
	val     T
	err     error
}

// start runs f unless a call is running already, and reports whether it
// did.
func (j *job[T]) start(f func() (T, error)) bool {
	j.mu.Lock()
	if j.running {
		j.mu.Unlock()
		return false
	}
	j.running, j.done = true, false
	j.mu.Unlock()

	spawn(func() {
		v, err := f()
		j.mu.Lock()
		j.val, j.err = v, err
		j.running, j.done = false, true
		j.mu.Unlock()
	})
	return true
}

// poll returns the result of the last call once, when it has finished.
func (j *job[T]) poll() (v T, err error, ok bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.done {
		return v, nil, false
	}
	j.done = false
	v, err = j.val, j.err
	var zero T
	j.val, j.err = zero, nil
	return v, err, true
}

// busy reports whether a call is running.
func (j *job[T]) busy() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.running
}
