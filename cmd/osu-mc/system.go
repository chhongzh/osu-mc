package main

import (
	"device/esp"
	"machine"
	"runtime"
	"strconv"
	"time"
	"unsafe"

	"osu-mc/internal/application"
)

// The memory map of the ESP32-S3, from esp32s3.ld: the data RAM and the
// windows through which the instruction RAM and the flash are seen.
const (
	dramStart = 0x3FC88000
	iramStart = 0x40378000
	dromStart = 0x3C000000
	iromStart = 0x42000000

	// romHeader is what the linker skips at the start of the data window
	// in flash: the image header and the header of the first segment.
	romHeader = 0x18 + 0x8

	// taskStack is the stack every goroutine but main is started with. It
	// is default-stack-size in esp32s3.json; the runtime does not say.
	taskStack = 8192

	// chipCores is how many Xtensa LX7 cores the chip has, whether or not
	// the runtime uses them.
	chipCores = 2
)

// The linker's symbols for where the parts of the firmware begin and end.
// Only their addresses mean anything.
var (
	//go:extern _heap_start
	heapStart [0]byte
	//go:extern _heap_end
	heapEnd [0]byte
	//go:extern _globals_start
	globalsStart [0]byte
	//go:extern _globals_end
	globalsEnd [0]byte
	//go:extern _edata
	dataEnd [0]byte
	//go:extern _stack_size
	stackSize [0]byte
	//go:extern _drom_end
	dromEnd [0]byte
	//go:extern _iram_end
	iramEnd [0]byte
	//go:extern _irom_end
	iromEnd [0]byte
)

func addr(p *[0]byte) uint32 { return uint32(uintptr(unsafe.Pointer(p))) }

// boot is when the firmware started, near enough: package variables are
// set before main runs.
var boot = time.Now()

// system is the System Info app's view of the board. The main loop reports
// each frame to it, so Stats can say how busy the loop is; both run on the
// UI goroutine, so it needs no lock.
type system struct {
	info    application.SystemInfo
	hasInfo bool

	// The loop over the second that is being counted, and over the last
	// whole one, which is what Stats reports.
	start        time.Time
	loops, draws int
	busy         time.Duration

	loopHz, drawHz, load int
}

func (s *system) Info() application.SystemInfo {
	if !s.hasInfo {
		s.info = readInfo()
		s.hasInfo = true
	}
	return s.info
}

func (s *system) Stats() application.SystemStats {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return application.SystemStats{
		Uptime:      int(time.Since(boot) / time.Second),
		Temp:        machine.ReadTemperature(),
		HeapUsed:    uint32(ms.HeapInuse),
		HeapFree:    uint32(ms.HeapIdle),
		GCMeta:      uint32(ms.GCSys),
		Objects:     uint32(ms.HeapObjects),
		Mallocs:     ms.Mallocs,
		Frees:       ms.Frees,
		TotalAlloc:  ms.TotalAlloc,
		Collections: ms.NumGC,
		LoopHz:      s.loopHz,
		DrawHz:      s.drawHz,
		Load:        s.load,
	}
}

// frame counts one pass of the main loop that began at start, drew if drew
// is set, and worked for busy before it went to sleep. Once a second has
// gone by the counts become the rates Stats reports.
func (s *system) frame(start time.Time, drew bool, busy time.Duration) {
	if s.start.IsZero() {
		s.start = start
	}
	s.loops++
	if drew {
		s.draws++
	}
	s.busy += busy
	if span := start.Sub(s.start); span >= time.Second {
		s.loopHz = int(int64(s.loops) * int64(time.Second) / int64(span))
		s.drawHz = int(int64(s.draws) * int64(time.Second) / int64(span))
		s.load = int(int64(s.busy) * 100 / int64(span))
		s.start, s.loops, s.draws, s.busy = start, 0, 0, 0
	}
}

// pause forgets the second being counted, for when the loop has been
// asleep with the screen off and that time is not a frame.
func (s *system) pause() {
	s.start, s.loops, s.draws, s.busy = time.Time{}, 0, 0, 0
}

// readInfo reads what does not change: the eFuses, the clocks and the
// linker's layout.
func readInfo() application.SystemInfo {
	// The eFuse block 1 words that hold the MAC and the chip's description,
	// numbered from bit 0 of the block as in the ESP-IDF eFuse table.
	w0 := esp.EFUSE.RD_MAC_SPI_SYS_0.Get()
	w1 := esp.EFUSE.RD_MAC_SPI_SYS_1.Get()
	w3 := esp.EFUSE.RD_MAC_SPI_SYS_3.Get()
	w4 := esp.EFUSE.RD_MAC_SPI_SYS_4.Get()
	w5 := esp.EFUSE.RD_MAC_SPI_SYS_5.Get()

	major := bits(w5, 24, 2)
	minor := bits(w5, 23, 1)<<3 | bits(w3, 18, 3)
	flashCap, flashVendor := bits(w3, 27, 3), bits(w4, 0, 3)
	psramCap := bits(w5, 19, 1)<<2 | bits(w4, 3, 2)
	psramVendor := bits(w4, 7, 2)

	mhz := 0
	if hz, err := machine.GetCPUFrequency(); err == nil {
		mhz = int(hz / 1e6)
	}

	irom, drom := addr(&iromEnd), addr(&dromEnd)
	// The instruction RAM sits behind the stack, the zeroed and the set
	// globals, which share the same SRAM through the data window.
	iram := addr(&iramEnd) - iramStart - (addr(&dataEnd) - dramStart)

	return application.SystemInfo{
		Chip:     esp.Device,
		Revision: "v" + strconv.Itoa(int(major)) + "." + strconv.Itoa(int(minor)),
		Package:  pick(bits(w3, 21, 3), "QFN56", "LGA56 (PICO-1)"),
		MAC: mac([6]byte{
			byte(w1 >> 8), byte(w1), byte(w0 >> 24), byte(w0 >> 16), byte(w0 >> 8), byte(w0),
		}),
		Cores:   chipCores,
		Used:    runtime.NumCPU(),
		CPUMHz:  mhz,
		Flash:   memory(pick(flashCap, "", "8 MB", "4 MB"), pick(flashVendor, "", "XMC", "GD", "FM", "TT", "BY"), "off chip"),
		PSRAM:   memory(pick(psramCap, "", "8 MB", "2 MB", "16 MB", "4 MB"), pick(psramVendor, "", "AP 3.3V", "AP 1.8V"), "none in chip"),
		Reset:   resetReason(esp.RTC_CNTL.GetRESET_STATE_RESET_CAUSE_PROCPU()),
		Runtime: "TinyGo " + runtime.Version(),

		SRAM:      addr(&heapEnd) - dramStart,
		Heap:      addr(&heapEnd) - addr(&heapStart),
		Globals:   addr(&globalsEnd) - addr(&globalsStart),
		Stack:     addr(&stackSize),
		TaskStack: taskStack,
		// The code in flash is linked to its place in the image, so where it
		// ends is where the image ends, but for the checksum and the padding.
		Image:  irom - iromStart,
		ROData: drom - dromStart - romHeader,
		IRAM:   iram,
	}
}

// bits returns n bits of w from bit at.
func bits(w uint32, at, n uint) uint32 {
	return w >> at & (1<<n - 1)
}

// pick returns the name for code, or "#code" for one it does not know.
func pick(code uint32, names ...string) string {
	if int(code) < len(names) {
		return names[code]
	}
	return "#" + strconv.Itoa(int(code))
}

// memory describes the flash or PSRAM in the chip's package: its size and
// maker, or none when the eFuses name no size.
func memory(size, vendor, none string) string {
	switch {
	case size == "":
		return none
	case vendor == "":
		return size
	}
	return size + " " + vendor
}

func mac(b [6]byte) string {
	const hex = "0123456789ABCDEF"
	var s [17]byte
	for i, c := range b {
		if i > 0 {
			s[i*3-1] = ':'
		}
		s[i*3], s[i*3+1] = hex[c>>4], hex[c&0xf]
	}
	return string(s[:])
}

// resetReason names the reset cause of core 0, as in ESP-IDF's
// soc/esp32s3/include/soc/reset_reasons.h.
func resetReason(code uint32) string {
	switch code {
	case 0x01:
		return "power on"
	case 0x03:
		return "software"
	case 0x05:
		return "deep sleep"
	case 0x07, 0x08, 0x0B, 0x11:
		return "watchdog"
	case 0x09, 0x0D, 0x10:
		return "RTC watchdog"
	case 0x0C:
		return "software (CPU)"
	case 0x0F:
		return "brown out"
	case 0x12:
		return "super watchdog"
	case 0x13:
		return "clock glitch"
	case 0x14:
		return "eFuse CRC"
	case 0x15:
		return "USB UART"
	case 0x16:
		return "USB JTAG"
	case 0x17:
		return "power glitch"
	}
	return "#" + strconv.Itoa(int(code))
}
