package animation

import "testing"

func TestCountLines(t *testing.T) {
	if got := countLines("a\nb\n"); got != 2 {
		t.Fatalf("got=%d", got)
	}
}

func TestIsTTY(t *testing.T) {
	_ = IsTTY()
}
