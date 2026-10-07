package ui

import (
	"image/color"

	"tinygo.org/x/tinyfont"
)

// Surface is what the UI draws on. *sh1106.Device satisfies it.
type Surface interface {
	Size() (int16, int16)
	SetPixel(x, y int16, c color.RGBA)
	InvertRect(x, y, w, h int16)
}

var white = color.RGBA{R: 255, G: 255, B: 255, A: 255}

// Canvas is a Surface with a clip rectangle and an origin. It is a small value
// type: narrow it with Within, move it with Translate and pass it down,
// nothing is allocated per frame.
//
// All coordinates passed to a canvas are local, relative to its origin.
type Canvas struct {
	s    Surface
	clip Rect // in surface coordinates
	ox   int16
	oy   int16
}

// NewCanvas returns a canvas covering the whole surface.
func NewCanvas(s Surface) Canvas {
	w, h := s.Size()
	return Canvas{s: s, clip: Rect{0, 0, w, h}}
}

// Clip returns the area the canvas may draw into, in local coordinates.
func (c Canvas) Clip() Rect {
	return Rect{c.clip.X - c.ox, c.clip.Y - c.oy, c.clip.W, c.clip.H}
}

// Within returns a canvas that is additionally clipped to r.
func (c Canvas) Within(r Rect) Canvas {
	r.X += c.ox
	r.Y += c.oy
	c.clip = c.clip.Intersect(r)
	return c
}

// Translate returns a canvas whose origin is moved by dx, dy. The clip stays
// where it is on the surface.
func (c Canvas) Translate(dx, dy int16) Canvas {
	c.ox += dx
	c.oy += dy
	return c
}

// Empty reports whether nothing drawn on c can be visible.
func (c Canvas) Empty() bool {
	return c.clip.Empty()
}

// Pixel turns on a single pixel.
func (c Canvas) Pixel(x, y int16) {
	x, y = x+c.ox, y+c.oy
	if x < c.clip.X || y < c.clip.Y || x >= c.clip.X+c.clip.W || y >= c.clip.Y+c.clip.H {
		return
	}
	c.s.SetPixel(x, y, white)
}

// FillRect turns on every pixel of the rectangle.
func (c Canvas) FillRect(x, y, w, h int16) {
	r := c.clip.Intersect(Rect{x + c.ox, y + c.oy, w, h})
	for j := r.Y; j < r.Y+r.H; j++ {
		for i := r.X; i < r.X+r.W; i++ {
			c.s.SetPixel(i, j, white)
		}
	}
}

// InvertRect flips every pixel of the rectangle.
func (c Canvas) InvertRect(x, y, w, h int16) {
	r := c.clip.Intersect(Rect{x + c.ox, y + c.oy, w, h})
	if !r.Empty() {
		c.s.InvertRect(r.X, r.Y, r.W, r.H)
	}
}

// InvertRoundRect flips every pixel of a rectangle with rounded corners of
// radius rad.
func (c Canvas) InvertRoundRect(x, y, w, h, rad int16) {
	rad = min(rad, w/2, h/2)
	if rad <= 0 {
		c.InvertRect(x, y, w, h)
		return
	}
	for j := int16(0); j < rad; j++ {
		in := cornerInset(rad, j)
		c.InvertRect(x+in, y+j, w-2*in, 1)
		c.InvertRect(x+in, y+h-1-j, w-2*in, 1)
	}
	c.InvertRect(x, y+rad, w, h-2*rad)
}

// cornerInset returns how far row j (counted from the outer edge) of a
// rounded corner of radius r is pulled in. The +r bias matches the midpoint
// circle algorithm, which gives the familiar pixel art corners: r=2 → 1,0 and
// r=3 → 2,1,0.
func cornerInset(r, j int16) int16 {
	dy := int32(r - j)
	return r - int16(isqrt(int32(r)*int32(r)-dy*dy+int32(r)))
}

func isqrt(v int32) int32 {
	if v <= 0 {
		return 0
	}
	x := int32(0)
	for (x+1)*(x+1) <= v {
		x++
	}
	return x
}

// DrawGlyph draws g with its origin (the left end of the baseline) at x, y.
//
// This is tinyfont's Glyph.Draw with clipping: rows outside the clip are
// skipped without touching the surface, and a glyph that is entirely outside
// costs one comparison.
func (c Canvas) DrawGlyph(g *tinyfont.Glyph, x, y int16) {
	gx, gy := x+c.ox+int16(g.XOffset), y+c.oy+int16(g.YOffset)
	w, h := int16(g.Width), int16(g.Height)
	cx0, cy0 := c.clip.X, c.clip.Y
	cx1, cy1 := cx0+c.clip.W, cy0+c.clip.H
	if gx >= cx1 || gy >= cy1 || gx+w <= cx0 || gy+h <= cy0 {
		return
	}

	bits := g.Bitmaps
	idx := 0
	for j := int16(0); j < h; j++ {
		py := gy + j
		if py < cy0 || py >= cy1 {
			idx += int(w)
			continue
		}
		for i := int16(0); i < w; i++ {
			if idx>>3 >= len(bits) {
				return
			}
			on := bits[idx>>3]&(0x80>>uint(idx&7)) != 0
			idx++
			if px := gx + i; on && px >= cx0 && px < cx1 {
				c.s.SetPixel(px, py, white)
			}
		}
	}
}
