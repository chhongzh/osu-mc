package application

import (
	"strconv"

	"osu-mc/internal/ui"
)

// systemPage is the System Info page: what the chip is, how the memory is
// used, how the firmware is laid out, how busy the main loop is, and the
// network. The lines that change are polled every statusPoll seconds; each
// is rebuilt only when its own value changed, so the strings are made at
// most once a second and never every frame.
type systemPage struct {
	*ui.List
	a *App

	// The lines that change; nil when the System service is missing.
	uptime, temp                         *ui.Text
	used, free, meta                     *ui.Text
	objects, gcs, mallocs, frees, totals *ui.Text
	loop, draw, load                     *ui.Text
	wifi, ip                             *ui.Text

	polled    bool // whether last and lastWiFi hold a poll yet
	last      SystemStats
	lastWiFi  WiFiStatus
	sincePoll float32
}

func (a *App) newSystem() *systemPage {
	p := &systemPage{a: a}
	items := []ui.ListItem{}

	if s := a.System; s != nil {
		info := s.Info()
		p.temp = ui.NewText("", nil)
		p.uptime = ui.NewText("", nil)
		items = append(items, heading("Chip"))
		items = addLine(items, info.Chip+" "+info.Revision, info.Chip != "")
		items = addLine(items, "Package: "+info.Package, info.Package != "")
		items = addLine(items, "CPU: "+strconv.Itoa(info.CPUMHz)+" MHz x"+strconv.Itoa(info.Cores), info.CPUMHz > 0)
		items = addLine(items, "Go uses "+plural(info.Used, "core"), info.Used > 0)
		items = append(items, p.temp)
		items = addLine(items, "Flash: "+info.Flash, info.Flash != "")
		items = addLine(items, "PSRAM: "+info.PSRAM, info.PSRAM != "")
		items = addLine(items, "MAC: "+info.MAC, info.MAC != "")
		items = addLine(items, "Reset: "+info.Reset, info.Reset != "")
		items = append(items, p.uptime)

		p.used = ui.NewText("", nil)
		p.free = ui.NewText("", nil)
		p.meta = ui.NewText("", nil)
		p.objects = ui.NewText("", nil)
		p.gcs = ui.NewText("", nil)
		p.mallocs = ui.NewText("", nil)
		p.frees = ui.NewText("", nil)
		p.totals = ui.NewText("", nil)
		items = append(items, heading("Memory"))
		items = addLine(items, "RAM: "+size(uint64(info.SRAM)), info.SRAM > 0)
		items = addLine(items, "Heap: "+size(uint64(info.Heap)), info.Heap > 0)
		items = append(items, p.used, p.free, p.meta, p.objects, p.gcs, p.mallocs, p.frees, p.totals)
		items = addLine(items, "Globals: "+size(uint64(info.Globals)), info.Globals > 0)
		items = addLine(items, "Stack: "+size(uint64(info.Stack)), info.Stack > 0)
		items = addLine(items, "Task stack: "+size(uint64(info.TaskStack)), info.TaskStack > 0)

		items = append(items, heading("Firmware"))
		items = addLine(items, info.Runtime, info.Runtime != "")
		items = addLine(items, "Image: "+size(uint64(info.Image)), info.Image > 0)
		items = addLine(items, "Constants: "+size(uint64(info.ROData)), info.ROData > 0)
		items = addLine(items, "RAM code: "+size(uint64(info.IRAM)), info.IRAM > 0)

		p.loop = ui.NewText("", nil)
		p.draw = ui.NewText("", nil)
		p.load = ui.NewText("", nil)
		items = append(items, heading("Main Loop"), p.loop, p.draw, p.load)
	} else {
		items = append(items, ui.NewText("System info n/a", nil))
	}

	p.wifi = ui.NewText("", nil)
	p.ip = ui.NewText("", nil)
	items = append(items, heading("Network"), p.wifi, p.ip)

	p.List = a.newList(items...)
	p.poll()
	return p
}

// Update polls the stats now and then.
func (p *systemPage) Update(dt float32) {
	p.List.Update(dt)
	p.sincePoll += dt
	if p.sincePoll >= statusPoll {
		p.sincePoll = 0
		p.poll()
	}
}

// poll reads the stats and the Wi-Fi status and rewrites the lines whose
// value changed. It builds strings, so it runs once a second at most.
func (p *systemPage) poll() {
	first := !p.polled
	p.polled = true

	if p.a.System != nil {
		n, o := p.a.System.Stats(), p.last
		p.last = n
		if first || n.Uptime != o.Uptime {
			p.uptime.SetText("Up: " + uptime(n.Uptime))
		}
		if first || n.Temp != o.Temp {
			p.temp.SetText("Temp: " + strconv.Itoa(int(roundDiv(n.Temp, 1000))) + " C")
		}
		if first || n.HeapUsed != o.HeapUsed || n.HeapFree != o.HeapFree {
			p.used.SetText("Used: " + size(uint64(n.HeapUsed)) + " " +
				strconv.Itoa(percent(n.HeapUsed, n.HeapUsed+n.HeapFree)) + "%")
			p.free.SetText("Free: " + size(uint64(n.HeapFree)))
		}
		if first || n.GCMeta != o.GCMeta {
			p.meta.SetText("GC meta: " + size(uint64(n.GCMeta)))
		}
		if first || n.Objects != o.Objects {
			p.objects.SetText("Objects: " + strconv.FormatUint(uint64(n.Objects), 10))
		}
		if first || n.Collections != o.Collections {
			p.gcs.SetText("GC runs: " + strconv.FormatUint(uint64(n.Collections), 10))
		}
		if first || n.Mallocs != o.Mallocs {
			p.mallocs.SetText("Allocs: " + strconv.FormatUint(n.Mallocs, 10))
		}
		if first || n.Frees != o.Frees {
			p.frees.SetText("Frees: " + strconv.FormatUint(n.Frees, 10))
		}
		if first || n.TotalAlloc != o.TotalAlloc {
			p.totals.SetText("Allocated: " + size(n.TotalAlloc))
		}
		if first || n.LoopHz != o.LoopHz {
			p.loop.SetText("Loop: " + strconv.Itoa(n.LoopHz) + " Hz")
		}
		if first || n.DrawHz != o.DrawHz {
			p.draw.SetText("Draw: " + strconv.Itoa(n.DrawHz) + " fps")
		}
		if first || n.Load != o.Load {
			p.load.SetText("Busy: " + strconv.Itoa(n.Load) + "%")
		}
	}

	if p.a.WiFi == nil {
		if first {
			p.wifi.SetText("Wi-Fi: n/a")
			p.ip.SetText("IP: -")
		}
		return
	}
	if s := p.a.WiFi.Status(); first || s != p.lastWiFi {
		p.lastWiFi = s
		if s.Connected {
			p.wifi.SetText("Wi-Fi: " + printable(s.SSID, false))
		} else {
			p.wifi.SetText("Wi-Fi: not connected")
		}
		if s.IP != "" {
			p.ip.SetText("IP: " + s.IP)
		} else {
			p.ip.SetText("IP: -")
		}
	}
}

// heading returns a section heading, which the selection passes over.
func heading(s string) *ui.Text {
	t := ui.NewText(s, nil)
	t.Static = true
	t.Center = true
	return t
}

// addLine adds a line for s if the value it shows is known, so that what
// the board cannot tell is left out rather than shown as zero.
func addLine(items []ui.ListItem, s string, known bool) []ui.ListItem {
	if !known {
		return items
	}
	return append(items, ui.NewText(printable(s, false), nil))
}

// size formats a number of bytes in the unit that keeps it short: "512 B",
// "9.5 KB", "267 KB", "1.27 MB". A KB is 1024 bytes. It rounds down, so a
// heap that is nearly full never shows as more than it is.
func size(n uint64) string {
	switch {
	case n < 1024:
		return strconv.FormatUint(n, 10) + " B"
	case n < 10*1024:
		return fixed(n*10/1024, 1) + " KB"
	case n < 1024*1024:
		return strconv.FormatUint(n/1024, 10) + " KB"
	default:
		return fixed(n*100/(1024*1024), 2) + " MB"
	}
}

// fixed formats v/10^digits with digits figures after the point.
func fixed(v uint64, digits int) string {
	s := strconv.FormatUint(v, 10)
	for len(s) <= digits {
		s = "0" + s
	}
	return s[:len(s)-digits] + "." + s[len(s)-digits:]
}

// uptime formats seconds as h:mm:ss, with the days in front once there are
// any.
func uptime(secs int) string {
	d, h, m, s := secs/86400, secs/3600%24, secs/60%60, secs%60
	out := strconv.Itoa(h) + ":" + two(m) + ":" + two(s)
	if d > 0 {
		out = strconv.Itoa(d) + "d " + out
	}
	return out
}

func two(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// percent returns part/whole in whole percent, rounded to the nearest.
func percent(part, whole uint32) int {
	if whole == 0 {
		return 0
	}
	return int((uint64(part)*100 + uint64(whole)/2) / uint64(whole))
}

// roundDiv returns a/b rounded to the nearest, halves away from zero, for
// a b above zero.
func roundDiv(a, b int32) int32 {
	if a < 0 {
		return -((-a + b/2) / b)
	}
	return (a + b/2) / b
}
