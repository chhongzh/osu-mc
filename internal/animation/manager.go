package animation

// Manager advances a set of values once per frame. Elements never step their
// own values, they only set targets; the render loop calls Update once.
type Manager struct {
	values []*Value

	// invalid forces the next Update to report a change, for state that
	// affects the picture but is not animated (for example new text).
	invalid bool
}

// Default is the manager that New and Value.Init register with.
var Default = &Manager{}

// Register adds v to the manager. Registering a value twice is a no-op.
func (m *Manager) Register(v *Value) {
	if v.idx >= 0 && int(v.idx) < len(m.values) && m.values[v.idx] == v {
		return
	}
	v.idx = int32(len(m.values))
	m.values = append(m.values, v)
}

// Unregister removes v from the manager, for elements that are thrown away.
func (m *Manager) Unregister(v *Value) {
	i := int(v.idx)
	if i < 0 || i >= len(m.values) || m.values[i] != v {
		return
	}
	last := len(m.values) - 1
	m.values[i] = m.values[last]
	m.values[i].idx = int32(i)
	m.values[last] = nil
	m.values = m.values[:last]
	v.idx = -1
}

// Invalidate makes the next Update report a change.
func (m *Manager) Invalidate() {
	m.invalid = true
}

// Update advances every value by dt seconds. It reports whether anything
// changed, so the caller can skip redrawing a frame that would look the same.
func (m *Manager) Update(dt float32) bool {
	changed := m.invalid
	m.invalid = false
	for _, v := range m.values {
		if v.step(dt) {
			changed = true
		}
	}
	return changed
}

// Len returns the number of registered values.
func (m *Manager) Len() int {
	return len(m.values)
}
