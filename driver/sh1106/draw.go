package sh1106

// Drawing primitives.
//
// Everything here writes into the frame buffer and marks the pages it touched;
// nothing reaches the bus until Display is called. Coordinates may fall
// outside the display, in which case the shape is clipped rather than
// rejected.
//
// The rectangle and span fills work on whole bytes: one buffer byte holds
// eight vertically stacked pixels, so a filled rectangle costs one read
// modify write per column per page instead of one per pixel.

// Clear fills the whole frame buffer with a single colour.
func (d *Device) Clear(c Color) {
	var v byte
	if c == On {
		v = 0xFF
	}
	for i := range d.buf {
		d.buf[i] = v
	}
	d.dirty = pageMask(d.pages)
}

// Pixel sets a single pixel. Coordinates outside the display are ignored.
func (d *Device) Pixel(x, y int16, c Color) {
	// The conversion to uint16 turns a negative coordinate into a large
	// value, so one comparison per axis covers both ends of the range.
	if uint16(x) >= uint16(d.width) || uint16(y) >= uint16(d.height) {
		return
	}

	pg := y >> 3
	i := pg*d.width + x
	bit := byte(1) << uint(y&7)

	if c == On {
		d.buf[i] |= bit
	} else {
		d.buf[i] &^= bit
	}
	d.dirty |= 1 << uint(pg)
}

// GetPixel reports the state of a pixel in the frame buffer. Coordinates
// outside the display read as Off.
func (d *Device) GetPixel(x, y int16) Color {
	if uint16(x) >= uint16(d.width) || uint16(y) >= uint16(d.height) {
		return Off
	}
	return d.buf[(y>>3)*d.width+x]&(1<<uint(y&7)) != 0
}

// FillRect fills the rectangle at (x, y) with the given size.
func (d *Device) FillRect(x, y, w, h int16, c Color) {
	x0, x1 := clampCols(x, w, d.width)
	y0, y1 := clampRows(y, h, d.height)
	if x0 >= x1 || y0 >= y1 {
		return
	}

	for pg := y0 >> 3; pg <= (y1-1)>>3; pg++ {
		// Rows of this page that the rectangle covers, as a bit mask.
		lo, hi := y0-(pg<<3), y1-(pg<<3)
		if lo < 0 {
			lo = 0
		}
		if hi > 8 {
			hi = 8
		}
		mask := byte(0xFF<<uint(lo)) & byte(0xFF>>uint(8-hi))

		row := d.buf[pg*d.width : (pg+1)*d.width]
		switch {
		case mask == 0xFF:
			// Whole page: a plain store, no read modify write.
			var v byte
			if c == On {
				v = 0xFF
			}
			for i := x0; i < x1; i++ {
				row[i] = v
			}
		case c == On:
			for i := x0; i < x1; i++ {
				row[i] |= mask
			}
		default:
			for i := x0; i < x1; i++ {
				row[i] &^= mask
			}
		}

		d.dirty |= 1 << uint(pg)
	}
}

// InvertRect flips every pixel in the rectangle at (x, y).
func (d *Device) InvertRect(x, y, w, h int16) {
	x0, x1 := clampCols(x, w, d.width)
	y0, y1 := clampRows(y, h, d.height)
	if x0 >= x1 || y0 >= y1 {
		return
	}

	for pg := y0 >> 3; pg <= (y1-1)>>3; pg++ {
		lo, hi := y0-(pg<<3), y1-(pg<<3)
		if lo < 0 {
			lo = 0
		}
		if hi > 8 {
			hi = 8
		}
		mask := byte(0xFF<<uint(lo)) & byte(0xFF>>uint(8-hi))

		row := d.buf[pg*d.width : (pg+1)*d.width]
		for i := x0; i < x1; i++ {
			row[i] ^= mask
		}

		d.dirty |= 1 << uint(pg)
	}
}

// Rect draws the outline of a rectangle at (x, y), one pixel wide.
func (d *Device) Rect(x, y, w, h int16, c Color) {
	if w <= 0 || h <= 0 {
		return
	}
	if w <= 2 || h <= 2 {
		// Too thin for an outline to have an interior.
		d.FillRect(x, y, w, h, c)
		return
	}

	d.FillRect(x, y, w, 1, c)
	d.FillRect(x, y+h-1, w, 1, c)
	d.FillRect(x, y+1, 1, h-2, c)
	d.FillRect(x+w-1, y+1, 1, h-2, c)
}

// HLine draws a horizontal line of w pixels starting at (x, y).
func (d *Device) HLine(x, y, w int16, c Color) {
	d.FillRect(x, y, w, 1, c)
}

// VLine draws a vertical line of h pixels starting at (x, y).
func (d *Device) VLine(x, y, h int16, c Color) {
	d.FillRect(x, y, 1, h, c)
}

// Line draws a line between the two points, both ends included.
func (d *Device) Line(x0, y0, x1, y1 int16, c Color) {
	// Axis aligned lines are common enough to be worth routing through the
	// byte level fill.
	if y0 == y1 {
		if x1 < x0 {
			x0, x1 = x1, x0
		}
		d.FillRect(x0, y0, x1-x0+1, 1, c)
		return
	}
	if x0 == x1 {
		if y1 < y0 {
			y0, y1 = y1, y0
		}
		d.FillRect(x0, y0, 1, y1-y0+1, c)
		return
	}

	// Bresenham.
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx := int16(1)
	if x0 > x1 {
		sx = -1
	}
	sy := int16(1)
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy

	for {
		d.Pixel(x0, y0, c)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := err << 1
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// Circle draws the outline of a circle of the given radius around (cx, cy).
func (d *Device) Circle(cx, cy, r int16, c Color) {
	if r < 0 {
		return
	}
	if r == 0 {
		d.Pixel(cx, cy, c)
		return
	}

	x, y := r, int16(0)
	err := 1 - r

	for x >= y {
		d.Pixel(cx+x, cy+y, c)
		d.Pixel(cx+y, cy+x, c)
		d.Pixel(cx-y, cy+x, c)
		d.Pixel(cx-x, cy+y, c)
		d.Pixel(cx-x, cy-y, c)
		d.Pixel(cx-y, cy-x, c)
		d.Pixel(cx+y, cy-x, c)
		d.Pixel(cx+x, cy-y, c)

		y++
		if err < 0 {
			err += 2*y + 1
		} else {
			x--
			err += 2*(y-x) + 1
		}
	}
}

// FillCircle fills a circle of the given radius around (cx, cy).
func (d *Device) FillCircle(cx, cy, r int16, c Color) {
	if r < 0 {
		return
	}

	x, y := r, int16(0)
	err := 1 - r

	for x >= y {
		// Each octant pair becomes one horizontal span.
		d.FillRect(cx-x, cy+y, 2*x+1, 1, c)
		d.FillRect(cx-x, cy-y, 2*x+1, 1, c)
		d.FillRect(cx-y, cy+x, 2*y+1, 1, c)
		d.FillRect(cx-y, cy-x, 2*y+1, 1, c)

		y++
		if err < 0 {
			err += 2*y + 1
		} else {
			x--
			err += 2*(y-x) + 1
		}
	}
}

// clampCols clips the column range x..x+w-1 to 0..limit.
func clampCols(x, w, limit int16) (int16, int16) {
	x0, x1 := x, x+w
	if x0 < 0 {
		x0 = 0
	}
	if x1 > limit {
		x1 = limit
	}
	return x0, x1
}

func abs(v int16) int16 {
	if v < 0 {
		return -v
	}
	return v
}
