package sh1106

// SH1106 command set.
//
// The SH1106 is close to the SSD1306 but not a drop-in replacement. Two
// differences matter here:
//
//   - DC-DC control is 0xAD followed by 0x8A/0x8B, not the SSD1306 charge-pump
//     pair 0x8D + 0x10/0x14. An SH1106 ignores 0x8D, which leaves the following
//     0x14 to be decoded as "set higher column address" (0x10..0x1F). That only
//     stays harmless because the column address is rewritten for every page.
//   - There is no MEMORYMODE / COLUMNADDR / PAGEADDR. Addressing is per page
//     with an explicit column address; the controller never wraps to the next
//     page on its own.
const (
	cmdSetLowColumn  = 0x00 // | lower 4 bits of the column address
	cmdSetHighColumn = 0x10 // | upper 4 bits of the column address
	cmdSetStartLine  = 0x40 // | display start line, 0..63
	cmdSetContrast   = 0x81 // + 1 byte
	cmdSegRemap      = 0xA0 // | 1 to mirror horizontally
	cmdDisplayResume = 0xA4
	cmdDisplayAllOn  = 0xA5
	cmdNormalDisplay = 0xA6
	cmdInvertDisplay = 0xA7
	cmdSetMultiplex  = 0xA8 // + 1 byte
	cmdDCDCControl   = 0xAD // + cmdDCDCOn / cmdDCDCOff
	cmdDCDCOff       = 0x8A
	cmdDCDCOn        = 0x8B
	cmdDisplayOff    = 0xAE
	cmdDisplayOn     = 0xAF
	cmdSetPageAddr   = 0xB0 // | page number, 0..7
	cmdComScanInc    = 0xC0
	cmdComScanDec    = 0xC8

	cmdSetDisplayOffset = 0xD3 // + 1 byte
	cmdSetClockDiv      = 0xD5 // + 1 byte
	cmdSetPrecharge     = 0xD9 // + 1 byte
	cmdSetComPins       = 0xDA // + 1 byte
	cmdSetVComDetect    = 0xDB // + 1 byte
)
