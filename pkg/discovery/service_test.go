package discovery

import "testing"

func TestOverlapUsesJaccardSimilarity(t *testing.T) {
	if got := overlap([]string{"go", "chat", "music"}, []string{"go", "music", "books"}); got != 0.5 {
		t.Fatalf("expected 0.5 Jaccard similarity, got %v", got)
	}
}

func TestNormalizeDeduplicatesAndSorts(t *testing.T) {
	got := normalize([]string{" Go ", "chat", "GO", "", "music"}, 10)
	want := []string{"chat", "go", "music"}
	if len(got) != len(want) {
		t.Fatalf("unexpected values: %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected values: %#v", got)
		}
	}
}
