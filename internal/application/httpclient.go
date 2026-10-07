package application

import "osu-mc/internal/ui"

// Shortcut is a saved request: a GET of URL, replayed from the HTTP
// client's list.
type Shortcut struct {
	URL string
}

// Shortcuts is the list of saved requests, in the order they were saved.
// They live in memory and are gone after a reboot.
type Shortcuts struct {
	list []*Shortcut
}

// Len returns the number of shortcuts.
func (s *Shortcuts) Len() int { return len(s.list) }

// At returns the i'th shortcut.
func (s *Shortcuts) At(i int) *Shortcut { return s.list[i] }

// Index returns the position of sc, or -1 if it was deleted.
func (s *Shortcuts) Index(sc *Shortcut) int {
	for i, x := range s.list {
		if x == sc {
			return i
		}
	}
	return -1
}

// Find returns the first shortcut for url, or nil.
func (s *Shortcuts) Find(url string) *Shortcut {
	for _, x := range s.list {
		if x.URL == url {
			return x
		}
	}
	return nil
}

// Add saves a new shortcut for url at the end and returns it.
func (s *Shortcuts) Add(url string) *Shortcut {
	sc := &Shortcut{URL: url}
	s.list = append(s.list, sc)
	return sc
}

// Delete removes sc and reports whether it was there.
func (s *Shortcuts) Delete(sc *Shortcut) bool {
	i := s.Index(sc)
	if i < 0 {
		return false
	}
	s.list = append(s.list[:i], s.list[i+1:]...)
	return true
}

// defaultURL is what the editor starts with: the scheme, which every URL
// needs and is the slowest part to type.
const defaultURL = "http://"

// httpHeader is the number of items above the shortcuts.
const httpHeader = 5

// httpPage is the HTTP client: an editor for one GET request, and the saved
// shortcuts below it. GET pushes the response; Save stores the URL as a new
// shortcut the first time and updates that same shortcut after that, so a
// request can be tweaked and sent again without piling up copies. Choosing
// a shortcut opens it, to replay, load, edit or delete it.
//
// The URL and the shortcut it belongs to are kept in the App, so leaving
// the page does not lose what was typed.
type httpPage struct {
	*ui.List
	a *App

	url             *ui.Input
	get, save, next *ui.Text
	heading         *ui.Text
	shown           []*Shortcut // the shortcuts below the header, in list order
}

func (a *App) newHTTP() *httpPage {
	if a.url == "" {
		a.url = defaultURL
	}
	p := &httpPage{
		a:       a,
		url:     ui.NewInput(a.Viewer, "URL"),
		get:     ui.NewText("GET", nil),
		save:    ui.NewText("", nil),
		next:    ui.NewText("New request", nil),
		heading: ui.NewText("", nil),
	}
	p.url.MaxLen = 128
	p.url.SetText(a.url)
	p.url.OnDone = func(s string) { a.url = s }
	p.heading.Static = true
	p.heading.Center = true

	p.List = a.newList(p.url, p.get, p.save, p.next, p.heading)
	p.OnSelect = p.choose
	p.showSave()
	p.sync()
	return p
}

func (p *httpPage) choose(i int) {
	a := p.a
	switch p.SelectedItem() {
	case p.get:
		a.Viewer.Push(a.newResponse(a.url))
	case p.save:
		p.saveURL()
	case p.next:
		p.load(defaultURL, nil)
	default:
		if i >= httpHeader && i-httpHeader < len(p.shown) {
			a.Viewer.Push(a.newShortcut(p, p.shown[i-httpHeader]))
		}
	}
}

// saveURL stores the URL: in the shortcut it was loaded from or last saved
// to, or else in a new one. Saving a URL that is saved already links the
// editor to that shortcut instead of adding a copy.
func (p *httpPage) saveURL() {
	a := p.a
	switch {
	case a.linked != nil:
		a.linked.URL = a.url
	case a.Shortcuts.Find(a.url) != nil:
		a.linked = a.Shortcuts.Find(a.url)
	default:
		a.linked = a.Shortcuts.Add(a.url)
	}
	p.showSave()
	p.sync()
}

// load puts url in the editor, linked to sc, which may be nil for a new
// request.
func (p *httpPage) load(url string, sc *Shortcut) {
	p.a.url = url
	p.a.linked = sc
	p.url.SetText(url)
	p.showSave()
}

func (p *httpPage) showSave() {
	if p.a.linked != nil {
		p.save.SetText("Update shortcut")
	} else {
		p.save.SetText("Save shortcut")
	}
}

// sync brings the shortcut lines in line with the App's shortcuts, the way
// the Wi-Fi page merges a scan: lines of deleted shortcuts close up, the
// others are relabeled in place and new ones are added at the end, so the
// selection stays where it was.
func (p *httpPage) sync() {
	s := &p.a.Shortcuts
	for i := len(p.shown) - 1; i >= 0; i-- {
		if s.Index(p.shown[i]) < 0 {
			p.shown = append(p.shown[:i], p.shown[i+1:]...)
			p.Remove(httpHeader + i)
		}
	}
	for i, sc := range p.shown {
		t := p.Item(httpHeader + i).(*ui.Text)
		if l := shortcutLabel(sc); t.Text() != l {
			t.SetText(l)
		}
	}
	for i := 0; i < s.Len(); i++ {
		if sc := s.At(i); indexShortcut(p.shown, sc) < 0 {
			p.shown = append(p.shown, sc)
			p.Add(ui.NewText(shortcutLabel(sc), nil))
		}
	}
	if len(p.shown) == 0 {
		p.heading.SetText("No shortcuts")
	} else {
		p.heading.SetText("Shortcuts")
	}
}

func indexShortcut(list []*Shortcut, sc *Shortcut) int {
	for i, x := range list {
		if x == sc {
			return i
		}
	}
	return -1
}

// shortcutLabel is how a shortcut is listed: its URL without the scheme,
// which is the same for nearly all of them and would only push the part
// that tells them apart off the screen.
func shortcutLabel(sc *Shortcut) string {
	u := sc.URL
	for _, scheme := range [...]string{"http://", "https://"} {
		if len(u) > len(scheme) && u[:len(scheme)] == scheme {
			u = u[len(scheme):]
			break
		}
	}
	return printable(u, false)
}

// newShortcut returns the page of one shortcut: replay it, load it into
// the editor, edit its URL or delete it. Delete asks to be chosen twice, so
// one stray press does not lose a shortcut.
func (a *App) newShortcut(editor *httpPage, sc *Shortcut) *ui.List {
	title := ui.NewText(shortcutLabel(sc), nil)
	title.Static = true
	title.Center = true

	replay := ui.NewText("Replay", nil)
	load := ui.NewText("Load in editor", nil)
	edit := ui.NewInput(a.Viewer, "URL")
	edit.MaxLen = 128
	edit.Format = func(s string) string { return "Edit: " + s }
	edit.SetText(sc.URL)
	del := ui.NewText("Delete", nil)

	edit.OnDone = func(s string) {
		sc.URL = s
		title.SetText(shortcutLabel(sc))
		editor.sync()
		if a.linked == sc {
			editor.load(s, sc)
		}
	}

	l := a.newList(title, replay, load, edit, del)
	l.OnSelect = func(int) {
		switch l.SelectedItem() {
		case replay:
			a.Viewer.Push(a.newResponse(sc.URL))
		case load:
			editor.load(sc.URL, sc)
			a.Viewer.Pop()
		case del:
			if del.Text() == "Delete" {
				del.SetText("Delete? Press again")
				return
			}
			a.Shortcuts.Delete(sc)
			if a.linked == sc {
				a.linked = nil
				editor.showSave()
			}
			editor.sync()
			a.Viewer.Pop()
		}
	}
	return l
}

// responsePage shows the response to a GET of url: "Loading..." until it
// is back, then the status line and the body.
type responsePage struct {
	*ui.TextView
	req job[Response]
	url string
}

func (a *App) newResponse(url string) *responsePage {
	p := &responsePage{
		TextView: a.newTextView("GET " + printable(url, false) + "\n\nLoading..."),
		url:      url,
	}
	if a.HTTP == nil {
		p.SetText("HTTP unavailable: connect to Wi-Fi in Settings.")
		return p
	}
	h := a.HTTP
	p.req.start(func() (Response, error) { return h.Get(url) })
	return p
}

// Update shows the response once it is back. The text is built only then,
// once, not every frame.
func (p *responsePage) Update(dt float32) {
	p.TextView.Update(dt)
	resp, err, ok := p.req.poll()
	if !ok {
		return
	}
	if err != nil {
		p.SetText("GET " + printable(p.url, false) + "\n\nError: " + printable(err.Error(), true))
		return
	}
	p.SetText(printable(resp.Status, false) + "\n\n" + printable(resp.Body, true))
}
