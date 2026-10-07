package sh1106

import "tinygo.org/x/drivers/pixel"

// DrawBitmap draws the given image at (x, y). It returns ErrBounds if the
// image does not fit entirely on the display.
//
// The two buffer layouts are transposed with respect to each other: a
// pixel.Image packs eight horizontally adjacent pixels into a byte, MSB first
// and with no padding between rows, while the display packs eight vertically
// adjacent pixels into a byte. Rather than copying pixel by pixel, the aligned
// case shuffles whole 8x8 blocks with the SWAR bit matrix transpose from
// Hacker's Delight, which turns 64 bit tests and 64 masked stores into three
// shift-and-mask rounds.
//
// The aligned path needs a source width and height that are multiples of 8 and
// a destination y on a page boundary. Anything else falls back to a slower but
// still direct bit copy. Chunked renderers such as tinygl hit the fast path
// whenever the chunk height is a multiple of 8.
func (d *Device) DrawBitmap(x, y int16, bmp pixel.Image[pixel.Monochrome]) error {
	w, h := bmp.Size()
	if x < 0 || y < 0 || x+int16(w) > d.width || y+int16(h) > d.height {
		return ErrBounds
	}
	if w == 0 || h == 0 {
		return nil
	}

	src := bmp.RawBuffer()
	if w%8 == 0 && h%8 == 0 && y%8 == 0 {
		d.blitAligned(x, y, int16(w), int16(h), src)
	} else {
		d.blitGeneric(x, y, int16(w), int16(h), src)
	}

	d.MarkDirty(y, int16(h))
	return nil
}

// blitAligned copies an image whose blocks line up with the display pages.
func (d *Device) blitAligned(x, y, w, h int16, src []byte) {
	stride := w / 8 // source bytes per row

	var block [8]byte
	for pg := int16(0); pg < h/8; pg++ {
		row := d.buf[(y/8+pg)*d.width:]
		srcRow := src[pg*8*stride:]

		for b := int16(0); b < stride; b++ {
			// Gather the eight source bytes that make up this 8x8 block:
			// one per image row, all at the same byte column.
			for j := int16(0); j < 8; j++ {
				block[j] = srcRow[j*stride+b]
			}
			transpose8(&block, row[x+b*8:])
		}
	}
}

// blitGeneric copies an image of any size to any position, one bit at a time.
func (d *Device) blitGeneric(x, y, w, h int16, src []byte) {
	for j := int16(0); j < h; j++ {
		// Rows are packed back to back with no padding, so a row does not
		// necessarily start on a byte boundary.
		base := int(j) * int(w)
		dstRow := d.buf[((y+j)>>3)*d.width:]
		bit := byte(1) << uint((y+j)&7)

		for i := int16(0); i < w; i++ {
			index := base + int(i)
			if src[index>>3]&(0x80>>uint(index&7)) != 0 {
				dstRow[x+i] |= bit
			} else {
				dstRow[x+i] &^= bit
			}
		}
	}
}

// transpose8 transposes an 8x8 bit matrix.
//
// Input byte src[r] holds row r with the leftmost pixel in bit 7. Output byte
// dst[c] holds column c with the topmost pixel in bit 0, which is the
// orientation the controller expects.
func transpose8(src *[8]byte, dst []byte) {
	var x uint64
	for i := 7; i >= 0; i-- {
		x = x<<8 | uint64(src[i])
	}

	x = x&0xAA55AA55AA55AA55 | (x&0x00AA00AA00AA00AA)<<7 | (x>>7)&0x00AA00AA00AA00AA
	x = x&0xCCCC3333CCCC3333 | (x&0x0000CCCC0000CCCC)<<14 | (x>>14)&0x0000CCCC0000CCCC
	x = x&0xF0F0F0F00F0F0F0F | (x&0x00000000F0F0F0F0)<<28 | (x>>28)&0x00000000F0F0F0F0

	for i := 7; i >= 0; i-- {
		dst[i] = byte(x)
		x >>= 8
	}
}
