// Package sh1106 implements a driver for the SH1106 monochrome OLED
// controller over SPI.
//
// The origin is the top left corner, x grows to the right and y grows
// downwards. The frame buffer is page organised: one byte holds eight
// vertically stacked pixels, least significant bit on top. That layout is
// exactly what the controller expects, so a full frame is a straight copy of
// the buffer onto the bus.
//
// Performance notes, relative to a straightforward implementation:
//
//   - The three address commands that precede each page are sent as one
//     transfer, which halves the number of Tx calls per frame.
//   - CS is asserted once per frame rather than once per transfer.
//   - Pages are tracked as dirty, so only the parts of the buffer that
//     actually changed are pushed.
//   - The drawing primitives write whole bytes per page instead of looping
//     over individual pixels.
//   - DrawBitmap transposes 8x8 pixel blocks with a SWAR bit shuffle rather
//     than reading the source image pixel by pixel.
package sh1106

import (
	"errors"
	"machine"
	"time"

	"tinygo.org/x/drivers"
)

// ErrBounds is returned when a drawing operation falls outside the display.
var ErrBounds = errors.New("sh1106: out of bounds")

// Color is the state of a pixel on a monochrome display.
type Color bool

// Pixel states.
const (
	Off Color = false
	On  Color = true
)

// Config holds the display configuration. The zero value describes the common
// 128x64 module.
type Config struct {
	// Width and Height of the panel in pixels. They default to 128x64.
	// Height must be a multiple of 8 and at most 64.
	Width, Height int16

	// ColOffset is the column offset from display RAM to the panel. The
	// SH1106 has 132 columns of RAM, so a centred 128 pixel panel starts at
	// column 2, which is also the default. Set it explicitly for panels
	// wired differently.
	ColOffset uint8

	// Contrast is the contrast level, 0..255. It defaults to 0xCF.
	Contrast uint8

	// ExternalVCC disables the internal DC-DC converter, for panels driven
	// from an external high voltage supply.
	ExternalVCC bool

	// Rotate180 turns the image upside down by flipping the segment remap
	// and the common scan direction. The panel does the work, so this costs
	// nothing at run time.
	Rotate180 bool
}

// Device is a SH1106 display.
type Device struct {
	bus drivers.SPI
	dc  machine.Pin
	rst machine.Pin
	cs  machine.Pin

	buf    []byte
	cmdbuf [3]byte

	width  int16
	height int16
	pages  int16

	colOffset uint8
	rotated   bool

	// dirty has one bit per page, set when the page has been modified since
	// the last call to Display.
	dirty uint8
}

// New creates a new SH1106 connection. The SPI bus must already be configured.
func New(bus drivers.SPI, dc, rst, cs machine.Pin) *Device {
	dc.Configure(machine.PinConfig{Mode: machine.PinOutput})
	rst.Configure(machine.PinConfig{Mode: machine.PinOutput})
	cs.Configure(machine.PinConfig{Mode: machine.PinOutput})

	return &Device{bus: bus, dc: dc, rst: rst, cs: cs}
}

// Configure resets the panel and sends the initialisation sequence.
func (d *Device) Configure(cfg Config) error {
	d.width = cfg.Width
	if d.width == 0 {
		d.width = 128
	}
	d.height = cfg.Height
	if d.height == 0 {
		d.height = 64
	}
	if d.height%8 != 0 || d.height > 64 {
		return errors.New("sh1106: height must be a multiple of 8 and at most 64")
	}

	d.colOffset = cfg.ColOffset
	if cfg.ColOffset == 0 {
		d.colOffset = 2
	}

	contrast := cfg.Contrast
	if contrast == 0 {
		contrast = 0xCF
	}

	d.pages = d.height / 8
	d.buf = make([]byte, int(d.width)*int(d.pages))

	d.reset()

	// Alternative COM pin configuration for 64 line panels, sequential below
	// that.
	comPins := byte(0x12)
	if d.height <= 32 {
		comPins = 0x02
	}

	segRemap := byte(cmdSegRemap | 0x01)
	comScan := byte(cmdComScanDec)
	d.rotated = cfg.Rotate180
	if cfg.Rotate180 {
		segRemap = cmdSegRemap
		comScan = cmdComScanInc
	}

	dcdc := byte(cmdDCDCOn)
	precharge := byte(0xF1)
	if cfg.ExternalVCC {
		dcdc = cmdDCDCOff
		precharge = 0x22
	}

	return d.Command(
		cmdDisplayOff,
		cmdSetClockDiv, 0x80,
		cmdSetMultiplex, byte(d.height-1),
		cmdSetDisplayOffset, 0x00,
		cmdSetStartLine|0x00,
		cmdDCDCControl, dcdc,
		segRemap,
		comScan,
		cmdSetComPins, comPins,
		cmdSetContrast, contrast,
		cmdSetPrecharge, precharge,
		cmdSetVComDetect, 0x40,
		cmdDisplayResume,
		cmdNormalDisplay,
		cmdDisplayOn,
	)
}

func (d *Device) reset() {
	d.cs.High()
	d.dc.Low()

	d.rst.High()
	time.Sleep(time.Millisecond)
	d.rst.Low()
	time.Sleep(10 * time.Millisecond)
	d.rst.High()
	time.Sleep(10 * time.Millisecond)
}

// Command sends a sequence of command bytes to the display.
func (d *Device) Command(cmds ...byte) error {
	d.cs.Low()
	d.dc.Low()
	err := d.bus.Tx(cmds, nil)
	d.cs.High()
	return err
}

// Size returns the current size of the display.
func (d *Device) Size() (x, y int16) {
	return d.width, d.height
}

// Buffer returns the frame buffer. Pixel (x, y) is bit y%8 of
// buf[(y/8)*width+x]. Call MarkDirty after writing to it directly, otherwise
// Display will not push the change.
func (d *Device) Buffer() []byte {
	return d.buf
}

// MarkDirty marks the pages covered by rows y..y+h-1 for retransmission. It is
// only needed when the frame buffer is modified through Buffer rather than
// through the drawing methods.
func (d *Device) MarkDirty(y, h int16) {
	if h <= 0 {
		return
	}
	y0, y1 := clampRows(y, h, d.height)
	if y0 >= y1 {
		return
	}
	for pg := y0 >> 3; pg <= (y1-1)>>3; pg++ {
		d.dirty |= 1 << uint(pg)
	}
}

// Display sends the modified pages of the frame buffer to the display. If
// nothing changed since the last call it returns without touching the bus.
func (d *Device) Display() error {
	if d.dirty == 0 {
		return nil
	}

	// One CS assertion for the whole frame; DC alone separates commands from
	// data.
	d.cs.Low()
	defer d.cs.High()

	for pg := int16(0); pg < d.pages; pg++ {
		if d.dirty&(1<<uint(pg)) == 0 {
			continue
		}

		d.cmdbuf[0] = cmdSetPageAddr | byte(pg)
		d.cmdbuf[1] = cmdSetLowColumn | (d.colOffset & 0x0F)
		d.cmdbuf[2] = cmdSetHighColumn | (d.colOffset >> 4)

		d.dc.Low()
		if err := d.bus.Tx(d.cmdbuf[:], nil); err != nil {
			return err
		}

		d.dc.High()
		if err := d.bus.Tx(d.buf[pg*d.width:(pg+1)*d.width], nil); err != nil {
			return err
		}

		d.dirty &^= 1 << uint(pg)
	}

	return nil
}

// DisplayFull sends the whole frame buffer, ignoring the dirty page tracking.
// Use it to recover from a display that lost data, for example after a
// glitch on the bus.
func (d *Device) DisplayFull() error {
	d.dirty = pageMask(d.pages)
	return d.Display()
}

// ClearDisplay clears the frame buffer and sends it to the display.
func (d *Device) ClearDisplay() error {
	d.Clear(Off)
	return d.Display()
}

// Sleep enters or leaves sleep mode. The frame buffer of the controller is
// retained while asleep.
func (d *Device) Sleep(enabled bool) error {
	if enabled {
		return d.Command(cmdDisplayOff)
	}
	return d.Command(cmdDisplayOn)
}

// SetContrast sets the contrast level, 0..255.
func (d *Device) SetContrast(level uint8) error {
	return d.Command(cmdSetContrast, level)
}

// InvertDisplay inverts the whole display in the panel, without touching the
// frame buffer.
func (d *Device) InvertDisplay(inverted bool) error {
	if inverted {
		return d.Command(cmdInvertDisplay)
	}
	return d.Command(cmdNormalDisplay)
}

// AllOn lights every pixel regardless of the frame buffer contents, which is
// useful to check the wiring. Call it with false to go back to showing the
// frame buffer.
func (d *Device) AllOn(on bool) error {
	if on {
		return d.Command(cmdDisplayAllOn)
	}
	return d.Command(cmdDisplayResume)
}

// SetStartLine sets the display start line, 0..63. Changing it scrolls the
// image vertically without rewriting the frame buffer.
func (d *Device) SetStartLine(line uint8) error {
	return d.Command(cmdSetStartLine | (line & 0x3F))
}

func pageMask(pages int16) uint8 {
	return uint8(1)<<uint(pages) - 1
}

// clampRows clips the row range y..y+h-1 to 0..limit.
func clampRows(y, h, limit int16) (int16, int16) {
	y0, y1 := y, y+h
	if y0 < 0 {
		y0 = 0
	}
	if y1 > limit {
		y1 = limit
	}
	return y0, y1
}
