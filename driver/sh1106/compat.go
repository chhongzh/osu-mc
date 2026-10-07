package sh1106

import (
	"image/color"

	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/pixel"
)

// Interface checks. The display satisfies both the color.RGBA based
// drivers.Displayer, which is what tinyfont and the older drawing helpers
// expect, and the pixel.Image based displayer used by tinygl. The latter is
// spelled out here rather than imported so this package keeps no dependency on
// tinygl.
var (
	_ drivers.Displayer = (*Device)(nil)

	_ interface {
		Size() (int16, int16)
		DrawBitmap(x, y int16, bitmap pixel.Image[pixel.Monochrome]) error
		Display() error
		Rotation() drivers.Rotation
	} = (*Device)(nil)
)

// SetPixel sets a pixel from an RGBA color, to satisfy drivers.Displayer. Any
// non black color turns the pixel on. Pixel is the cheaper entry point when
// the caller already knows the color is monochrome.
func (d *Device) SetPixel(x, y int16, c color.RGBA) {
	d.Pixel(x, y, c.R|c.G|c.B != 0)
}

// Rotation returns the rotation of the display.
//
// Rotate180 in the configuration is carried out by the panel itself, by
// flipping the segment remap and the common scan direction, so the frame
// buffer coordinate system is unchanged either way. The value is reported
// because it also determines which way the display scrolls.
func (d *Device) Rotation() drivers.Rotation {
	if d.rotated {
		return drivers.Rotation180
	}
	return drivers.Rotation0
}

// RGBA converts a monochrome pixel to black or white.
func (c Color) RGBA() color.RGBA {
	if c {
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	return color.RGBA{A: 255}
}
