package application

import (
	"strconv"

	"osu-mc/internal/ui"
)

// newSettings returns the settings page: Wi-Fi, turning the screen off and
// about.
func (a *App) newSettings() *ui.List {
	title := ui.NewText("Settings", nil)
	title.Static = true
	title.Center = true

	wifi := ui.NewText("Wi-Fi", nil)
	screen := ui.NewText("Screen off", nil)
	if a.Power == nil {
		screen.SetText("Screen off (n/a)")
	}
	about := ui.NewText("About", nil)

	l := a.newList(title, wifi, screen, about)
	l.OnSelect = func(int) {
		switch l.SelectedItem() {
		case wifi:
			a.Viewer.Push(a.newWiFi())
		case screen:
			if a.Power != nil {
				a.Power.ScreenOff()
			}
		case about:
			a.Viewer.Push(a.newTextView(aboutText))
		}
	}
	return l
}

const aboutText = "osu!mc\n" +
	"A small osu!(lazer) style UI for a 128x64 OLED.\n" +
	"\n" +
	"Developer: chhongzh\n" +
	"\n" +
	"TinyGo on ESP32-S3, SH1106 display, six keys.\n" +
	"Press back to return."

// Timings of the Wi-Fi page, in seconds.
const (
	// statusPoll is how often the page asks for the status, so that a
	// connection that drops shows up without a key press.
	statusPoll = 1
	// rescanEvery is how long the page waits after a scan before it scans
	// again, while it is on top and not joining a network.
	rescanEvery = 15
)

// wifiHeader is the number of items above the access points.
const wifiHeader = 3

// wifiPage is the Wi-Fi page: the status, the IP address and a scan line,
// then the access points the last scan found. It scans when it opens and
// again every rescanEvery seconds, updating the list in place: networks
// that are still there keep their line and selection, new ones are added at
// the end and those that are gone close up. Choosing one asks for the
// password on a keyboard and joins it.
type wifiPage struct {
	*ui.List
	a *App

	state, ip, scan *ui.Text
	aps             []AP // the networks below the header, in list order

	scanJob    job[[]AP]
	connectJob job[struct{}]
	joining    string // the SSID being joined, while connectJob runs
	failure    string // why the last scan or join failed, until the next

	last      WiFiStatus
	sincePoll float32
	sinceScan float32
}

func (a *App) newWiFi() *wifiPage {
	p := &wifiPage{
		a:     a,
		state: ui.NewText("", nil),
		ip:    ui.NewText("", nil),
		scan:  ui.NewText("Scan", nil),
	}
	p.List = a.newList(p.state, p.ip, p.scan)
	p.OnSelect = p.choose
	if a.WiFi == nil {
		p.state.SetText("Wi-Fi unavailable")
		p.ip.SetText("IP: -")
		return p
	}
	p.last = a.WiFi.Status()
	p.showStatus()
	p.startScan()
	return p
}

func (p *wifiPage) choose(i int) {
	if p.a.WiFi == nil {
		return
	}
	switch {
	case p.SelectedItem() == p.scan:
		p.startScan()
	case i >= wifiHeader && i-wifiHeader < len(p.aps):
		p.askPassword(p.aps[i-wifiHeader].SSID)
	}
}

// askPassword pushes a keyboard for the password of ssid. Its OK key joins
// the network; going back from it cancels.
func (p *wifiPage) askPassword(ssid string) {
	if p.connectJob.busy() {
		return
	}
	v := p.a.Viewer
	k := ui.NewKeyboard()
	k.MaxLen = 63 // the longest WPA2 passphrase
	k.Placeholder = "Password"
	k.SetTarget(p.a.page())
	k.SetText(p.a.password(ssid))
	k.OnDone = func(pass string) {
		if v.Top() == ui.Element(k) {
			v.Pop()
		}
		p.connect(ssid, pass)
	}
	v.Push(k)
}

func (p *wifiPage) connect(ssid, pass string) {
	w := p.a.WiFi
	started := p.connectJob.start(func() (struct{}, error) {
		return struct{}{}, w.Connect(ssid, pass)
	})
	if !started {
		return
	}
	p.a.remember(ssid, pass)
	p.joining = ssid
	p.failure = ""
	p.showStatus()
}

func (p *wifiPage) startScan() {
	w := p.a.WiFi
	if p.scanJob.start(w.Scan) {
		p.scan.SetText("Scanning...")
	}
}

// Update polls the scan and the join, and the status now and then.
func (p *wifiPage) Update(dt float32) {
	p.List.Update(dt)
	if p.a.WiFi == nil {
		return
	}

	if aps, err, ok := p.scanJob.poll(); ok {
		p.sinceScan = 0
		if err != nil {
			p.failure = "Scan: " + err.Error()
			p.showStatus()
		} else {
			p.merge(aps)
		}
		p.scan.SetText("Rescan (" + strconv.Itoa(len(p.aps)) + " found)")
	}

	if _, err, ok := p.connectJob.poll(); ok {
		if err != nil {
			p.failure = "Failed: " + err.Error()
		}
		p.joining = ""
		p.last = p.a.WiFi.Status()
		p.showStatus()
	}

	p.sincePoll += dt
	if p.sincePoll >= statusPoll {
		p.sincePoll = 0
		if s := p.a.WiFi.Status(); s != p.last {
			p.last = s
			p.showStatus()
		}
	}

	// It rescans only while it is on top: not under the password keyboard,
	// and not while joining, when a scan would fight over the radio.
	if p.a.Viewer.Top() == ui.Element(p) {
		p.sinceScan += dt
	}
	if p.sinceScan >= rescanEvery && p.joining == "" && !p.scanJob.busy() {
		p.sinceScan = 0
		p.startScan()
	}
}

// showStatus writes the status lines. It builds strings, so it runs when
// something changed, never every frame.
func (p *wifiPage) showStatus() {
	switch {
	case p.joining != "":
		p.state.SetText("Joining " + printable(p.joining, false) + "...")
	case p.failure != "":
		p.state.SetText(printable(p.failure, false))
	case p.last.Connected:
		p.state.SetText("Connected: " + printable(p.last.SSID, false))
	default:
		p.state.SetText("Not connected")
	}
	if p.last.IP != "" {
		p.ip.SetText("IP: " + p.last.IP)
	} else {
		p.ip.SetText("IP: -")
	}
}

// merge brings the list in line with a new scan. Hidden networks, which
// have no name to join them by, are left out, and a network that several
// access points share is listed once with the strongest signal.
func (p *wifiPage) merge(found []AP) {
	found = strongest(found)

	// Drop the networks that are gone, from the end so the indices of the
	// ones still to look at do not move.
	for i := len(p.aps) - 1; i >= 0; i-- {
		if indexAP(found, p.aps[i].SSID) < 0 {
			p.aps = append(p.aps[:i], p.aps[i+1:]...)
			p.Remove(wifiHeader + i)
		}
	}
	// Update the ones still there.
	for i := range p.aps {
		j := indexAP(found, p.aps[i].SSID)
		if found[j].RSSI != p.aps[i].RSSI {
			p.aps[i].RSSI = found[j].RSSI
			p.Item(wifiHeader + i).(*ui.Text).SetText(apLabel(p.aps[i]))
		}
	}
	// Add the new ones, strongest first.
	for _, ap := range found {
		if indexAP(p.aps, ap.SSID) < 0 {
			p.aps = append(p.aps, ap)
			p.Add(ui.NewText(apLabel(ap), nil))
		}
	}
}

// strongest returns the named networks of aps, each once with its best
// signal, strongest first.
func strongest(aps []AP) []AP {
	var out []AP
	for _, ap := range aps {
		if ap.SSID == "" {
			continue
		}
		if j := indexAP(out, ap.SSID); j >= 0 {
			out[j].RSSI = max(out[j].RSSI, ap.RSSI)
			continue
		}
		out = append(out, ap)
	}
	// Insertion sort: a scan finds a handful of networks.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].RSSI > out[j-1].RSSI; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func indexAP(aps []AP, ssid string) int {
	for i, ap := range aps {
		if ap.SSID == ssid {
			return i
		}
	}
	return -1
}

// apLabel is how a network is listed: its name, with '?' for what the font
// cannot draw, and the signal in dBm.
func apLabel(ap AP) string {
	return printable(ap.SSID, false) + " " + strconv.Itoa(ap.RSSI)
}

// password returns the password last used for ssid, to start the keyboard
// with.
func (a *App) password(ssid string) string {
	return a.passwords[ssid]
}

func (a *App) remember(ssid, pass string) {
	if a.passwords == nil {
		a.passwords = make(map[string]string)
	}
	a.passwords[ssid] = pass
}
