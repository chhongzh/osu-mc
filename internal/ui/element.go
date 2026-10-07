package ui

// Element is a node of the UI.
//
// Update runs logic and sets animation targets; it must not step animated
// values itself, animation.Default does that once per frame after every
// Update. Draw renders the current values.
type Element interface {
	Update(dt float32)
	Draw(c Canvas)
}

// Group updates and draws its elements in order, so later ones are on top.
type Group []Element

func (g Group) Update(dt float32) {
	for _, e := range g {
		e.Update(dt)
	}
}

func (g Group) Draw(c Canvas) {
	for _, e := range g {
		e.Draw(c)
	}
}
