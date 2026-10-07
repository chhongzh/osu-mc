package application

import "osu-mc/internal/ui"

// The Lorem Ipsum app is a mock of osu!'s song select: three levels of
// pages, songs, difficulties and options, that do nothing but show off the
// UI.

var songs = []string{
	"Camellia - GHOST",
	"xi - FREEDOM DiVE",
	"Hige Driver - Miracle Sugite Yabai (feat. Ayane)",
	"cYsmix - Peer Gynt",
	"DragonForce - Through the Fire and Flames",
	"TheFatRat - Unity",
	"Knife Party - Centipede",
	"LeaF - Aleph-0",
	"Sakuzyo - Altale",
	"Laur - Sound Chimera",
}

var difficulties = []string{
	"[Easy] 1.92*",
	"[Normal] 2.85*",
	"[Hard] 3.97*",
	"[Insane] 5.24*",
	"[Expert] 6.61*",
	"[Extra] 7.38*",
}

var (
	mods  = []string{"EZ", "NF", "HT", "HR", "SD", "PF", "DT", "NC", "HD", "FL", "RX", "AP", "SO"}
	rates = []string{"0.5x", "0.75x", "1.0x", "1.25x", "1.5x", "2.0x"}
)

// newLorem returns the song list: a search field, the songs and a reader.
// Enter on the search field opens a keyboard; its OK key jumps to the first
// song that matches, going back from it leaves the search as it was.
func (a *App) newLorem() *ui.List {
	search := ui.NewInput(a.Viewer, "Search")
	search.MaxLen = 24
	search.Format = func(s string) string { return "Search: " + s }

	home := a.newList(newLabels(songs...)...)
	home.Insert(0, search)
	reader := ui.NewText("Reader", nil)
	home.Add(reader)

	search.OnDone = func(q string) {
		if i := findSong(q); i >= 0 {
			home.Select(i + 1)
		}
	}
	home.OnSelect = func(i int) {
		switch home.SelectedItem() {
		case search:
			// The input takes enter itself; this is only a safety net.
		case reader:
			a.Viewer.Push(a.newTextView(readerText))
		default:
			diffs := a.newList(newLabels(difficulties...)...)
			diffs.OnSelect = func(int) {
				a.Viewer.Push(a.newOptions())
			}
			a.Viewer.Push(diffs)
		}
	}
	return home
}

// newOptions returns the options page: a heading and rows that scroll
// sideways. Up and down move between the rows, passing over the heading;
// left at the start of a row goes back.
func (a *App) newOptions() *ui.List {
	title := ui.NewText("Options", nil)
	title.Center = true
	title.Static = true

	// Mods are a multiple choice: HD and DT can be on at once.
	modRow := ui.NewToggles(mods...)

	// The rate is a single choice: the bar slides over to the new one.
	rateRow := ui.NewChoice(rates...)
	rateRow.Choose(2)
	rateRow.Select(2)

	actions := ui.NewRow(newLabels("Play", "Autoplay", "Leaderboard", "Back")...)
	actions.OnSelect = func(i int) {
		if i == 3 {
			a.Viewer.Back()
		}
	}

	l := a.newList(title, modRow, rateRow, actions)
	l.Gap = 0
	l.Offset = nil // the rows span the screen and scroll themselves
	l.Marquee = false
	l.OnRight = nil // the rows scroll with it
	return l
}

// readerText is long enough to scroll both ways: prose that wraps, and
// lines that only the unwrapped view shows running off to the right.
const readerText = "osu!mc reader\n" +
	"Up and down scroll by a line. With wrapping on, left and right turn the page; with it off they scroll sideways. Enter switches between the two.\n" +
	"\n" +
	"Rank  Player          Acc     PP     Combo\n" +
	"#1    mrekk           99.12%  1438   2391x\n" +
	"#2    lifeline        98.87%  1402   2390x\n" +
	"#3    Accolibed       98.65%  1377   2388x\n" +
	"\n" +
	"Press back or RST to return."

// findSong returns the first song whose title contains q, ignoring ASCII
// case, or -1. An empty q matches nothing.
func findSong(q string) int {
	if q == "" {
		return -1
	}
	for i, s := range songs {
		if containsFold(s, q) {
			return i
		}
	}
	return -1
}

// containsFold reports whether s contains sub, ignoring ASCII case. It
// does the case folding by hand so that the strings package and its
// Unicode tables stay out of the firmware.
func containsFold(s, sub string) bool {
	for j := 0; j+len(sub) <= len(s); j++ {
		if equalFold(s[j:j+len(sub)], sub) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if lower(a[i]) != lower(b[i]) {
			return false
		}
	}
	return true
}

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}
