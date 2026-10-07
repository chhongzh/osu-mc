package animation

import (
	"math"
	"testing"
)

func TestExpNeg(t *testing.T) {
	for x := float32(0); x < 20; x += 0.037 {
		want := math.Exp(-float64(x))
		if got := float64(expNeg(x)); math.Abs(got-want) > 1e-4 {
			t.Fatalf("expNeg(%v) = %v, want %v", x, got, want)
		}
	}
}

// The same animation stepped at different frame rates should be in the same
// place after the same amount of time.
func TestFrameRateIndependent(t *testing.T) {
	pos := func(fps int) float32 {
		m := &Manager{}
		v := &Value{Speed: 10, idx: -1}
		m.Register(v)
		v.Set(100)
		for i := 0; i < fps/5; i++ { // 200 ms
			m.Update(1 / float32(fps))
		}
		return v.Current
	}
	a, b := pos(30), pos(120)
	if math.Abs(float64(a-b)) > 0.5 {
		t.Fatalf("30 fps: %v, 120 fps: %v", a, b)
	}
}

func TestSettles(t *testing.T) {
	m := &Manager{}
	v := &Value{Speed: 20, idx: -1}
	m.Register(v)
	v.Set(-37)
	for i := 0; i < 200 && m.Update(1.0/120); i++ {
	}
	if !v.Settled() || v.Current != -37 {
		t.Fatalf("not settled: %v", v.Current)
	}
	if m.Update(1.0 / 120) {
		t.Fatal("settled manager reports a change")
	}
	m.Invalidate()
	if !m.Update(1.0 / 120) {
		t.Fatal("invalidated manager reports no change")
	}
}

func TestUnregister(t *testing.T) {
	m := &Manager{}
	vs := []*Value{{idx: -1}, {idx: -1}, {idx: -1}}
	for _, v := range vs {
		m.Register(v)
	}
	m.Register(vs[1]) // no-op
	m.Unregister(vs[0])
	m.Unregister(vs[0]) // no-op
	if m.Len() != 2 || m.values[vs[2].idx] != vs[2] || m.values[vs[1].idx] != vs[1] {
		t.Fatalf("bad state after unregister: %d values", m.Len())
	}
}

func TestRound(t *testing.T) {
	for _, c := range []struct {
		in  float32
		out int16
	}{{0.4, 0}, {0.5, 1}, {-0.4, 0}, {-0.5, -1}, {-1.6, -2}} {
		if got := Round(c.in); got != c.out {
			t.Errorf("Round(%v) = %v, want %v", c.in, got, c.out)
		}
	}
}

func TestSpring(t *testing.T) {
	run := func(damping float32) (peak float32, settled bool) {
		m := &Manager{}
		v := &Value{Speed: 20, Damping: damping, idx: -1}
		m.Register(v)
		v.Set(100)
		for i := 0; i < 240 && m.Update(1.0/120); i++ {
			peak = max(peak, v.Current)
		}
		return peak, v.Settled() && v.Current == 100
	}
	if peak, ok := run(1); !ok || peak > 100 {
		t.Fatalf("critically damped: peak %v, settled %v", peak, ok)
	}
	if peak, ok := run(0.4); !ok || peak < 110 {
		t.Fatalf("underdamped: peak %v, settled %v, want an overshoot", peak, ok)
	}
}

func TestKick(t *testing.T) {
	m := &Manager{}
	v := &Value{Speed: 20, Damping: 1, idx: -1}
	m.Register(v)
	v.Kick(8)
	var peak float32
	for i := 0; i < 240 && m.Update(1.0/120); i++ {
		peak = max(peak, v.Current)
	}
	if peak < 6 || peak > 9 || v.Current != 0 {
		t.Fatalf("kick of 8 peaked at %v and ended at %v", peak, v.Current)
	}
}
