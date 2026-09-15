package matcher

import "testing"

func TestNormalizeLowercasesAndCollapsesPunctuation(t *testing.T) {
	cases := map[string]string{
		"DOUBLE-DEAL (Live)":  "double deal live",
		"  Hello,   World!  ": "hello world",
		"Haruka Mirai":        "haruka mirai",
		"神崎エルザ":               "神崎エルザ",
		"":                    "",
	}
	for input, want := range cases {
		if got := Normalize(input); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeAppliesUnicodeNFC(t *testing.T) {
	nfc := "Café"
	nfd := "Cafe\u0301" // e + combining acute accent (macOS style)
	if Normalize(nfc) != Normalize(nfd) {
		t.Errorf("Normalize(NFC)=%q != Normalize(NFD)=%q", Normalize(nfc), Normalize(nfd))
	}
}

func TestSimilarityIsOneForIdenticalStrings(t *testing.T) {
	if got := Similarity("abc", "abc"); got != 1 {
		t.Errorf("Similarity(abc, abc) = %v, want 1", got)
	}
}

func TestSimilarityIsZeroWhenOneSideIsEmpty(t *testing.T) {
	if got := Similarity("abc", ""); got != 0 {
		t.Errorf("Similarity(abc, \"\") = %v, want 0", got)
	}
}

func TestSimilarityHandlesTinyTypos(t *testing.T) {
	got := Similarity("harukamirai", "haruka mirai")
	if got < 0.85 {
		t.Errorf("Similarity = %v, want at least 0.85", got)
	}
}

func TestMatchPrefersExactTitleArtistAlbum(t *testing.T) {
	index := NewIndex([]Track{
		{ID: "wrong", Title: "Disorder", ArtistName: "Someone", AlbumTitle: "Other"},
		{ID: "right", Title: "Disorder", ArtistName: "神崎エルザ", AlbumTitle: "Elza"},
	})

	track, ok := index.Match("Disorder", "神崎エルザ", "Elza")
	if !ok {
		t.Fatal("Match returned ok=false, want a match")
	}
	if track.ID != "right" {
		t.Errorf("matched %q, want right", track.ID)
	}
}

func TestMatchUsesAlbumToBreakTies(t *testing.T) {
	index := NewIndex([]Track{
		{ID: "a", Title: "Again", ArtistName: "milet", AlbumTitle: "inside you EP"},
		{ID: "b", Title: "Again", ArtistName: "milet", AlbumTitle: "other"},
	})

	track, ok := index.Match("Again", "milet", "inside you EP")
	if !ok {
		t.Fatal("Match returned ok=false, want a match")
	}
	if track.ID != "a" {
		t.Errorf("matched %q, want a", track.ID)
	}
}

func TestMatchFindsNearMissWithoutAlbum(t *testing.T) {
	index := NewIndex([]Track{
		{ID: "haruka", Title: "Haruka Mirai", ArtistName: "KANKAKU PIERO", AlbumTitle: "Iroirotoiro"},
	})

	track, ok := index.Match("Harukamirai", "KANKAKU PIERO", "")
	if !ok {
		t.Fatal("Match returned ok=false, want a match")
	}
	if track.ID != "haruka" {
		t.Errorf("matched %q, want haruka", track.ID)
	}
}

func TestMatchRejectsUnrelatedTracks(t *testing.T) {
	index := NewIndex([]Track{
		{ID: "x", Title: "Completely Different", ArtistName: "Another Artist", AlbumTitle: "Other"},
	})

	if _, ok := index.Match("Disorder", "神崎エルザ", "Elza"); ok {
		t.Error("Match returned ok=true, want no match")
	}
}

func TestMatchIsDeterministicForEqualScores(t *testing.T) {
	index := NewIndex([]Track{
		{ID: "b", Title: "Song", ArtistName: "Artist", AlbumTitle: "Album"},
		{ID: "a", Title: "Song", ArtistName: "Artist", AlbumTitle: "Album"},
	})

	first, _ := index.Match("Song", "Artist", "Album")
	second, _ := index.Match("Song", "Artist", "Album")
	if first.ID != second.ID {
		t.Errorf("matches differ between calls: %q vs %q", first.ID, second.ID)
	}
}

func TestLenCountsIndexedTracks(t *testing.T) {
	index := NewIndex([]Track{{ID: "a"}, {ID: "b"}})
	if got := index.Len(); got != 2 {
		t.Errorf("Len() = %d, want 2", got)
	}
}
