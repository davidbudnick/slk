package threadsview

import "testing"

// BenchmarkViewScroll simulates j/k scrolling through a long threads list
// where only m.selected changes between View() calls.
func BenchmarkViewScroll(b *testing.B) {
	m := New(names(), "USELF")
	m.SetSummaries(manySummaries(200))
	_ = m.View(40, 80)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			m.MoveUp()
		} else {
			m.MoveDown()
		}
		_ = m.View(40, 80)
	}
}

// BenchmarkViewFocusFlip1000 is the h/l focus toggle in the Threads view
// at a large terminal (420x121) with the ~1000 subscribed threads a busy
// workspace accumulates. Each flip re-renders the panel; only the cards
// in the viewport may be drawn, or this takes hundreds of milliseconds.
func BenchmarkViewFocusFlip1000(b *testing.B) {
	m := New(names(), "USELF")
	m.SetSummaries(manySummaries(1000))
	_ = m.View(119, 235)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.SetFocused(i%2 == 0)
		_ = m.View(119, 235)
	}
}

// BenchmarkViewJumpEnds1000 is G then gg across ~1000 threads: the jump
// must lay out only the cards around its target.
func BenchmarkViewJumpEnds1000(b *testing.B) {
	m := New(names(), "USELF")
	m.SetSummaries(manySummaries(1000))
	_ = m.View(119, 235)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			m.GoToBottom()
		} else {
			m.GoToTop()
		}
		_ = m.View(119, 235)
	}
}
