package application

import (
	"errors"
	"image/color"
	"strings"
	"testing"

	"osu-mc/internal/animation"
	"osu-mc/internal/ui"
)

func init() {
	// Blocking calls run in place, so their result is there on the next
	// Update.
	spawn = func(f func()) { f() }
}

// screen is a 1 bit frame buffer standing in for the display.
type screen struct {
	w, h int16
	px   []bool
}

func newScreen(w, h int16) *screen {
	return &screen{w: w, h: h, px: make([]bool, int(w)*int(h))}
}

func (s *screen) Size() (int16, int16) { return s.w, s.h }

func (s *screen) SetPixel(x, y int16, c color.RGBA) {
	if x < 0 || y < 0 || x >= s.w || y >= s.h {
		panic("SetPixel outside the screen")
	}
	s.px[int(y)*int(s.w)+int(x)] = c.R|c.G|c.B != 0
}

func (s *screen) InvertRect(x, y, w, h int16) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			s.px[int(j)*int(s.w)+int(i)] = !s.px[int(j)*int(s.w)+int(i)]
		}
	}
}

func (s *screen) String() string {
	var b strings.Builder
	for y := int16(0); y < s.h; y++ {
		for x := int16(0); x < s.w; x++ {
			if s.px[int(y)*int(s.w)+int(x)] {
				b.WriteByte('#')
			} else {
				b.WriteByte('.')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func frame(e ui.Element) *screen {
	s := newScreen(128, 64)
	e.Draw(ui.NewCanvas(s))
	return s
}

func run(e ui.Element, secs float32) {
	const dt = 1.0 / 120
	for t := float32(0); t < secs; t += dt {
		e.Update(dt)
		animation.Default.Update(dt)
	}
}

type fakeWiFi struct {
	aps      []AP
	scans    int
	joined   []string // ssid/password of each Connect
	fail     error
	status   WiFiStatus
	scanFail error
}

func (w *fakeWiFi) Scan() ([]AP, error) {
	w.scans++
	return append([]AP(nil), w.aps...), w.scanFail
}

func (w *fakeWiFi) Connect(ssid, pass string) error {
	w.joined = append(w.joined, ssid+"/"+pass)
	if w.fail != nil {
		return w.fail
	}
	w.status = WiFiStatus{Connected: true, SSID: ssid, IP: "192.168.1.7"}
	return nil
}

func (w *fakeWiFi) Status() WiFiStatus { return w.status }

type fakeHTTP struct {
	got  []string
	resp Response
	err  error
}

func (h *fakeHTTP) Get(url string) (Response, error) {
	h.got = append(h.got, url)
	return h.resp, h.err
}

type fakePower struct{ off int }

func (p *fakePower) ScreenOff() { p.off++ }

type fakeSystem struct {
	info  SystemInfo
	stats SystemStats
	reads int
}

func (s *fakeSystem) Info() SystemInfo { return s.info }

func (s *fakeSystem) Stats() SystemStats {
	s.reads++
	return s.stats
}

// harness is an App on a 128x64 viewer with a router, like the firmware.
type harness struct {
	t      *testing.T
	app    *App
	viewer *ui.Viewer
	router *ui.Router
}

func newHarness(t *testing.T, s Services) *harness {
	v := ui.NewViewer(128)
	v.Height = 64
	h := &harness{t: t, app: New(v, s), viewer: v, router: &ui.Router{Root: v}}
	h.app.Start()
	h.run(1)
	t.Cleanup(func() {
		v.PopToRoot()
		h.run(1)
		if r, ok := v.Top().(ui.Releaser); ok {
			r.Release()
		}
		animation.Default.Update(0)
	})
	return h
}

func (h *harness) run(secs float32) { run(h.viewer, secs) }

func (h *harness) tap(k ui.Key) {
	h.router.Dispatch(ui.Event{Key: k, Phase: ui.Press})
	h.router.Dispatch(ui.Event{Key: k, Phase: ui.Release})
	h.run(0.5)
}

// open moves the selection of the top list to the item labelled label and
// chooses it.
func (h *harness) open(label string) {
	h.t.Helper()
	l := h.list()
	for i := 0; i < l.Len(); i++ {
		if t, ok := l.Item(i).(*ui.Text); ok && t.Text() == label {
			l.Select(i)
			h.tap(ui.KeyEnter)
			return
		}
	}
	h.t.Fatalf("no %q in %v", label, h.labels())
}

// list returns the top page as a list.
func (h *harness) list() *ui.List {
	h.t.Helper()
	switch p := h.viewer.Top().(type) {
	case *ui.List:
		return p
	case *wifiPage:
		return p.List
	case *httpPage:
		return p.List
	case *systemPage:
		return p.List
	}
	h.t.Fatalf("top page is %T, not a list", h.viewer.Top())
	return nil
}

func (h *harness) labels() []string {
	l := h.list()
	var out []string
	for i := 0; i < l.Len(); i++ {
		switch it := l.Item(i).(type) {
		case *ui.Text:
			out = append(out, it.Text())
		case *ui.Input:
			out = append(out, it.Field().Text())
		}
	}
	return out
}

// keyboard returns the keyboard on top.
func (h *harness) keyboard() *ui.Keyboard {
	h.t.Helper()
	k, ok := h.viewer.Top().(*ui.Keyboard)
	if !ok {
		h.t.Fatalf("top page is %T, not a keyboard", h.viewer.Top())
	}
	return k
}

func TestPrintable(t *testing.T) {
	for _, c := range []struct {
		in   string
		text bool
		want string
	}{
		{"plain", false, "plain"},
		{"咖啡店", false, "???"},
		{"Café 5G", false, "Caf? 5G"},
		{"a\tb\nc", false, "a?b?c"},
		{"a\tb\nc", true, "a\tb\nc"},
		{"bell\x07", true, "bell?"},
		{"\xff", false, "?"},
	} {
		if got := printable(c.in, c.text); got != c.want {
			t.Errorf("printable(%q, %v) = %q, want %q", c.in, c.text, got, c.want)
		}
	}
}

func TestShortcuts(t *testing.T) {
	var s Shortcuts
	a := s.Add("http://a")
	b := s.Add("http://b")
	if s.Len() != 2 || s.At(0) != a || s.Index(b) != 1 || s.Find("http://b") != b {
		t.Fatal("add/find")
	}
	if !s.Delete(a) || s.Delete(a) || s.Len() != 1 || s.Index(a) != -1 {
		t.Fatal("delete")
	}
}

func TestLauncher(t *testing.T) {
	h := newHarness(t, Services{})
	if got := h.labels(); strings.Join(got, ",") != "osu!mc,Settings,Lorem Ipsum,HTTP Client,System Info" {
		t.Fatalf("launcher = %v", got)
	}
	h.open("Lorem Ipsum")
	h.open("Camellia - GHOST")
	h.open("[Hard] 3.97*")
	if h.viewer.Len() != 4 {
		t.Fatalf("depth = %d, want 4", h.viewer.Len())
	}
	h.tap(ui.KeyBack)
	h.tap(ui.KeyBack)
	h.tap(ui.KeyBack)
	if h.viewer.Len() != 1 {
		t.Fatalf("depth after back = %d, want 1", h.viewer.Len())
	}
	t.Logf("launcher:\n%s", frame(h.viewer))
}

func TestLoremSearch(t *testing.T) {
	h := newHarness(t, Services{})
	h.open("Lorem Ipsum")
	h.list().Select(0) // the search field, above the first song
	h.tap(ui.KeyEnter)
	h.keyboard().OnDone("peer")
	h.run(1)
	l := h.list()
	if got := l.SelectedItem().(*ui.Text).Text(); got != "cYsmix - Peer Gynt" {
		t.Fatalf("selected %q after search", got)
	}
}

func TestScreenOff(t *testing.T) {
	p := &fakePower{}
	h := newHarness(t, Services{Power: p})
	h.open("Settings")
	h.open("Screen off")
	if p.off != 1 {
		t.Fatalf("ScreenOff called %d times", p.off)
	}

	h2 := newHarness(t, Services{})
	h2.open("Settings")
	h2.open("Screen off (n/a)") // does nothing, and does not crash
}

func TestAbout(t *testing.T) {
	h := newHarness(t, Services{})
	h.open("Settings")
	h.open("About")
	v, ok := h.viewer.Top().(*ui.TextView)
	if !ok || !strings.Contains(v.Text(), "chhongzh") {
		t.Fatalf("about page is %T", h.viewer.Top())
	}
}

func TestWiFi(t *testing.T) {
	w := &fakeWiFi{aps: []AP{
		{"home", -60}, {"", -40}, {"cafe", -70}, {"home", -50}, {"咖啡", -80},
	}}
	h := newHarness(t, Services{WiFi: w})
	h.open("Settings")
	h.open("Wi-Fi")
	got := strings.Join(h.labels(), ",")
	want := "Not connected,IP: -,Rescan (3 found),home -50,cafe -70,?? -80"
	if got != want {
		t.Fatalf("wifi page = %s\nwant          %s", got, want)
	}
	t.Logf("wifi:\n%s", frame(h.viewer))

	// Join cafe: the keyboard asks for the password, OK joins.
	h.open("cafe -70")
	h.keyboard().OnDone("secret")
	h.run(1)
	if len(w.joined) != 1 || w.joined[0] != "cafe/secret" {
		t.Fatalf("joined %v", w.joined)
	}
	if got := h.labels()[:2]; got[0] != "Connected: cafe" || got[1] != "IP: 192.168.1.7" {
		t.Fatalf("status = %v", got)
	}

	// The keyboard remembers the password.
	h.open("cafe -70")
	if got := h.keyboard().Text(); got != "secret" {
		t.Fatalf("keyboard starts with %q", got)
	}
	w.fail = errors.New("auth")
	h.keyboard().OnDone("wrong")
	h.run(1)
	if got := h.labels()[0]; got != "Failed: auth" {
		t.Fatalf("status after failure = %q", got)
	}

	// A rescan drops what is gone and keeps the selection on what is not.
	h.list().Select(4) // cafe
	w.aps = []AP{{"cafe", -65}, {"new", -55}}
	h.open("Rescan (3 found)")
	h.run(1)
	got = strings.Join(h.labels(), ",")
	want = "Failed: auth,IP: 192.168.1.7,Rescan (2 found),cafe -65,new -55"
	if got != want {
		t.Fatalf("after rescan = %s\nwant           %s", got, want)
	}

	// It rescans on its own while it is on top.
	n := w.scans
	h.run(rescanEvery + 1)
	if w.scans != n+1 {
		t.Fatalf("scans = %d, want %d", w.scans, n+1)
	}
}

func TestWiFiUnavailable(t *testing.T) {
	h := newHarness(t, Services{})
	h.open("Settings")
	h.open("Wi-Fi")
	if got := h.labels()[0]; got != "Wi-Fi unavailable" {
		t.Fatalf("status = %q", got)
	}
	h.open("Scan") // does nothing
}

func TestHTTP(t *testing.T) {
	c := &fakeHTTP{resp: Response{Status: "200 OK", Body: "héllo\nworld"}}
	h := newHarness(t, Services{HTTP: c})
	h.open("HTTP Client")
	got := strings.Join(h.labels(), ",")
	if got != "http://,GET,Save shortcut,New request,No shortcuts" {
		t.Fatalf("http page = %s", got)
	}

	// Type a URL and GET it.
	h.tap(ui.KeyEnter)
	h.keyboard().OnDone("http://example.com/a")
	h.run(1)
	h.open("GET")
	v := h.viewer.Top().(*responsePage)
	h.run(0.1)
	if v.Text() != "200 OK\n\nh?llo\nworld" {
		t.Fatalf("response = %q", v.Text())
	}
	t.Logf("response:\n%s", frame(h.viewer))
	h.tap(ui.KeyBack)

	// The first save adds a shortcut, the next updates it.
	h.open("Save shortcut")
	if h.app.Shortcuts.Len() != 1 {
		t.Fatal("not saved")
	}
	h.tap(ui.KeyUp)
	h.tap(ui.KeyUp)
	h.tap(ui.KeyEnter)
	h.keyboard().OnDone("http://example.com/b")
	h.run(1)
	h.open("Update shortcut")
	got = strings.Join(h.labels(), ",")
	if got != "http://example.com/b,GET,Update shortcut,New request,Shortcuts,example.com/b" {
		t.Fatalf("after update = %s", got)
	}

	// A new request saves a second shortcut.
	h.open("New request")
	h.list().Select(0)
	h.tap(ui.KeyEnter)
	h.keyboard().OnDone("http://other")
	h.run(1)
	h.open("Save shortcut")
	if h.app.Shortcuts.Len() != 2 {
		t.Fatalf("shortcuts = %d", h.app.Shortcuts.Len())
	}

	// The editor survives leaving the page.
	h.tap(ui.KeyBack)
	h.open("HTTP Client")
	if got := h.labels()[0]; got != "http://other" {
		t.Fatalf("editor = %q", got)
	}

	// Replay the first shortcut.
	h.open("example.com/b")
	h.open("Replay")
	h.run(0.1)
	if c.got[len(c.got)-1] != "http://example.com/b" {
		t.Fatalf("replayed %v", c.got)
	}
	h.tap(ui.KeyBack)

	// Edit it.
	h.list().Select(3)
	h.tap(ui.KeyEnter)
	h.keyboard().OnDone("http://example.com/c")
	h.run(1)
	if h.app.Shortcuts.At(0).URL != "http://example.com/c" {
		t.Fatal("edit did not update the shortcut")
	}

	// Load it, which links the editor to it.
	h.open("Load in editor")
	if got := h.labels(); got[0] != "http://example.com/c" || got[2] != "Update shortcut" {
		t.Fatalf("after load = %v", got)
	}

	// Delete it: twice, the first only asks.
	h.open("example.com/c")
	h.open("Delete")
	if h.app.Shortcuts.Len() != 2 {
		t.Fatal("deleted on the first press")
	}
	h.open("Delete? Press again")
	h.run(1)
	got = strings.Join(h.labels(), ",")
	if got != "http://example.com/c,GET,Save shortcut,New request,Shortcuts,other" {
		t.Fatalf("after delete = %s", got)
	}

	// Errors are shown too.
	c.err = errors.New("no route")
	h.open("GET")
	h.run(0.1)
	if v := h.viewer.Top().(*responsePage); !strings.HasSuffix(v.Text(), "Error: no route") {
		t.Fatalf("error = %q", v.Text())
	}
}

func TestHTTPUnavailable(t *testing.T) {
	h := newHarness(t, Services{})
	h.open("HTTP Client")
	h.open("GET")
	h.run(0.1)
	if v := h.viewer.Top().(*responsePage); !strings.HasPrefix(v.Text(), "HTTP unavailable") {
		t.Fatalf("text = %q", v.Text())
	}
}

func TestSystem(t *testing.T) {
	sys := &fakeSystem{
		info: SystemInfo{
			Chip: "ESP32-S3", Revision: "v0.2", Package: "QFN56",
			MAC: "AA:BB:CC:DD:EE:FF", Cores: 2, Used: 1, CPUMHz: 240,
			Flash: "8 MB GD", Reset: "power on", Runtime: "TinyGo 0.42.0",
			SRAM: 407312, Heap: 274276, Globals: 82344, Stack: 4096,
			TaskStack: 8192, Image: 1344683, ROData: 233288, IRAM: 46596,
		},
		stats: SystemStats{
			Uptime: 3725, Temp: 41600,
			HeapUsed: 40960, HeapFree: 221184, GCMeta: 8192, Objects: 321,
			Mallocs: 1000, Frees: 679, TotalAlloc: 3 << 20, Collections: 4,
			LoopHz: 120, DrawHz: 60, Load: 7,
		},
	}
	w := &fakeWiFi{status: WiFiStatus{Connected: true, SSID: "home", IP: "192.168.1.7"}}
	h := newHarness(t, Services{WiFi: w, System: sys})
	h.open("System Info")
	got := strings.Join(h.labels(), ",")
	want := "System Info,Chip,ESP32-S3 v0.2,Package: QFN56,CPU: 240 MHz x2,Go uses 1 core," +
		"Temp: 42 C,Flash: 8 MB GD,MAC: AA:BB:CC:DD:EE:FF,Reset: power on,Up: 1:02:05," +
		"Memory,RAM: 397 KB,Heap: 267 KB,Used: 40 KB 16%,Free: 216 KB,GC meta: 8.0 KB," +
		"Objects: 321,GC runs: 4,Allocs: 1000,Frees: 679,Allocated: 3.00 MB," +
		"Globals: 80 KB,Stack: 4.0 KB,Task stack: 8.0 KB," +
		"Firmware,TinyGo 0.42.0,Image: 1.28 MB,Constants: 227 KB,RAM code: 45 KB," +
		"Main Loop,Loop: 120 Hz,Draw: 60 fps,Busy: 7%," +
		"Network,Wi-Fi: home,IP: 192.168.1.7"
	if got != want {
		t.Fatalf("system page = %s\nwant           %s", got, want)
	}
	t.Logf("system:\n%s", frame(h.viewer))

	// It polls about once a second and rewrites only what changed.
	reads := sys.reads
	sys.stats.Uptime = 90061
	sys.stats.Temp = -1500
	w.status = WiFiStatus{}
	h.run(1.1)
	if sys.reads != reads+1 {
		t.Fatalf("reads = %d, want %d", sys.reads, reads+1)
	}
	got = strings.Join(h.labels(), ",")
	for _, s := range []string{"Up: 1d 1:01:01", "Temp: -2 C", "Wi-Fi: not connected", "IP: -"} {
		if !strings.Contains(got, s) {
			t.Errorf("no %q in %s", s, got)
		}
	}
}

func TestSystemUnavailable(t *testing.T) {
	h := newHarness(t, Services{})
	h.open("System Info")
	got := strings.Join(h.labels(), ",")
	want := "System Info,System info n/a,Network,Wi-Fi: n/a,IP: -"
	if got != want {
		t.Fatalf("system page = %s, want %s", got, want)
	}
	h.run(2) // polls with nothing to poll, and does not crash
}

func TestSize(t *testing.T) {
	for _, c := range []struct {
		n    uint64
		want string
	}{
		{0, "0 B"}, {1023, "1023 B"}, {1024, "1.0 KB"}, {9727, "9.4 KB"},
		{10 * 1024, "10 KB"}, {274276, "267 KB"}, {1 << 20, "1.00 MB"},
		{1344683, "1.28 MB"},
	} {
		if got := size(c.n); got != c.want {
			t.Errorf("size(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}
