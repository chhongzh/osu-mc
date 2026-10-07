package application

import "unicode/utf8"

// Services are what the apps need from the hardware. Any of them may be
// nil, the apps then say the feature is not available.
//
// The blocking calls are made off the UI goroutine, one at a time per page;
// the implementations must still be safe to call from several goroutines,
// as a page that was left may have a call running while a new one starts.
type Services struct {
	WiFi   WiFi
	HTTP   HTTP
	Power  Power
	System System
}

// AP is an access point found by a scan.
type AP struct {
	SSID string
	RSSI int // dBm
}

// WiFiStatus is the state of the station interface.
type WiFiStatus struct {
	Connected bool
	SSID      string
	IP        string // dotted quad, empty without an address
}

// WiFi is the station interface.
type WiFi interface {
	// Scan blocks until a scan has finished.
	Scan() ([]AP, error)

	// Connect blocks until the station has joined ssid and has an
	// address, or has given up. An empty password joins an open network.
	Connect(ssid, password string) error

	// Status returns the state right now, without blocking.
	Status() WiFiStatus
}

// Response is what a request came back with.
type Response struct {
	Status string // the status line, like "200 OK"
	Body   string // cut short by the implementation if it is long
}

// HTTP makes requests.
type HTTP interface {
	// Get blocks until the response has been read.
	Get(url string) (Response, error)
}

// Power controls the screen.
type Power interface {
	// ScreenOff turns the screen off, from the end of this frame until
	// any key is pressed. The press that wakes it is not passed on.
	ScreenOff()
}

// System reports on the board and the firmware running on it.
type System interface {
	// Info returns what does not change while the firmware runs.
	Info() SystemInfo

	// Stats returns what does. Reading the heap walks the allocator's
	// metadata under its lock, so it is called about once a second, not
	// every frame.
	Stats() SystemStats
}

// SystemInfo describes the chip and how the firmware is laid out in it.
// Sizes are in bytes; a zero size or an empty string is not known.
type SystemInfo struct {
	Chip     string // the model, like "ESP32-S3"
	Revision string // the silicon revision, like "v0.2"
	Package  string // the package, like "QFN56"
	MAC      string // the factory MAC address, colon separated
	Cores    int    // the cores the chip has
	Used     int    // the cores the runtime schedules goroutines on
	CPUMHz   int
	Flash    string // the flash inside the package, if any
	PSRAM    string // the PSRAM inside the package, if any
	Reset    string // why the chip last started
	Runtime  string // the compiler and its version

	SRAM      uint32 // the data RAM the firmware may use
	Heap      uint32 // the garbage collected heap, metadata included
	Globals   uint32 // the global variables
	Stack     uint32 // the stack of the main goroutine
	TaskStack uint32 // the stack every other goroutine gets
	Image     uint32 // the firmware image in flash
	ROData    uint32 // the constants in flash
	IRAM      uint32 // the code that runs from RAM
}

// SystemStats is the state of the board right now.
type SystemStats struct {
	Uptime int   // seconds since boot
	Temp   int32 // the die temperature in millidegrees Celsius

	HeapUsed    uint32 // bytes in live objects
	HeapFree    uint32 // bytes free for new objects
	GCMeta      uint32 // bytes the collector keeps for itself
	Objects     uint32 // live objects
	Mallocs     uint64 // objects allocated since boot
	Frees       uint64 // objects freed since boot
	TotalAlloc  uint64 // bytes allocated since boot
	Collections uint32 // garbage collections since boot

	// The main loop over the last second: how often it ran, how often it
	// drew a frame, and the share of the time it was not asleep, in
	// percent. Network goroutines run while it sleeps.
	LoopHz, DrawHz, Load int
}

// printable returns s with every character the font cannot draw replaced
// by '?': anything outside ASCII, one '?' per character rather than per
// byte, and control characters. With text, newlines, tabs and carriage
// returns are kept for a TextView to lay out.
func printable(s string, text bool) string {
	clean := true
	for i := 0; i < len(s); i++ {
		if !keep(s[i], text) {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			if keep(c, text) {
				b = append(b, c)
			} else {
				b = append(b, '?')
			}
			i++
			continue
		}
		// An invalid sequence decodes one byte at a time, so it shows as
		// a '?' per byte, which is as good a guess as any.
		_, n := utf8.DecodeRuneInString(s[i:])
		b = append(b, '?')
		i += n
	}
	return string(b)
}

func keep(c byte, text bool) bool {
	switch {
	case c >= ' ' && c < 0x7f:
		return true
	case text:
		return c == '\n' || c == '\t' || c == '\r'
	}
	return false
}
