package matcher

// Track is a library track in the shape the matcher needs.
type Track struct {
	ID              string
	Title           string
	AlbumID         string
	AlbumTitle      string
	ArtistID        string
	ArtistName      string
	DiscNumber      int
	TrackNumber     int
	DurationSeconds float64
	Format          string
}

// Matching thresholds. Title similarity is required separately so that a very
// close artist/album can never compensate for a completely different title.
const (
	minTitleSimilarity = 0.70
	minMatchScore      = 0.80
)

// Index provides fast lookup of library tracks by normalized metadata.
type Index struct {
	tracks             []Track
	byTitleArtistAlbum map[string]Track
	byTitleArtist      map[string][]Track
	byArtist           map[string][]Track
	byTitleInitial     map[string][]Track
}

// NewIndex builds an Index over the given library tracks.
func NewIndex(tracks []Track) *Index {
	index := &Index{
		tracks:             tracks,
		byTitleArtistAlbum: make(map[string]Track, len(tracks)),
		byTitleArtist:      make(map[string][]Track, len(tracks)),
		byArtist:           make(map[string][]Track, len(tracks)),
		byTitleInitial:     make(map[string][]Track, len(tracks)),
	}

	for _, track := range tracks {
		title := Normalize(track.Title)
		artist := Normalize(track.ArtistName)
		album := Normalize(track.AlbumTitle)
		index.byTitleArtistAlbum[albumKey(title, artist, album)] = track
		index.byTitleArtist[artistKey(title, artist)] = append(index.byTitleArtist[artistKey(title, artist)], track)
		index.byArtist[artist] = append(index.byArtist[artist], track)
		if initial := firstRune(title); initial != "" {
			index.byTitleInitial[initial] = append(index.byTitleInitial[initial], track)
		}
	}
	return index
}

// Len returns the number of indexed tracks.
func (i *Index) Len() int { return len(i.tracks) }

// Match returns the best library track for the given metadata. The second
// result is false when nothing scores high enough.
func (i *Index) Match(title, artist, album string) (Track, bool) {
	normalizedTitle := Normalize(title)
	normalizedArtist := Normalize(artist)
	normalizedAlbum := Normalize(album)

	if track, ok := i.byTitleArtistAlbum[albumKey(normalizedTitle, normalizedArtist, normalizedAlbum)]; ok {
		return track, true
	}
	if tracks := i.byTitleArtist[artistKey(normalizedTitle, normalizedArtist)]; len(tracks) > 0 {
		return bestMatch(normalizedTitle, normalizedArtist, normalizedAlbum, tracks)
	}
	candidates := i.candidates(normalizedTitle, normalizedArtist)
	if len(candidates) == 0 {
		return Track{}, false
	}
	return bestMatch(normalizedTitle, normalizedArtist, normalizedAlbum, candidates)
}

// candidates narrows the fuzzy search to tracks by the same artist, or failing
// that, tracks whose title starts with the same character. This keeps matching
// fast even for very large libraries.
func (i *Index) candidates(title, artist string) []Track {
	seen := make(map[string]struct{})
	var candidates []Track

	for _, track := range i.byArtist[artist] {
		seen[track.ID] = struct{}{}
		candidates = append(candidates, track)
	}
	for _, track := range i.byTitleInitial[firstRune(title)] {
		if _, ok := seen[track.ID]; ok {
			continue
		}
		seen[track.ID] = struct{}{}
		candidates = append(candidates, track)
	}
	return candidates
}

func bestMatch(title, artist, album string, candidates []Track) (Track, bool) {
	var best Track
	bestScore := 0.0
	found := false
	for _, candidate := range candidates {
		score, ok := matchScore(title, artist, album, candidate)
		if !ok {
			continue
		}
		if !found || score > bestScore || (score == bestScore && candidate.ID < best.ID) {
			best, bestScore, found = candidate, score, true
		}
	}
	if !found || bestScore < minMatchScore {
		return Track{}, false
	}
	return best, true
}

// matchScore combines title/artist/(album) similarity. It returns ok=false when
// the title itself is not close enough.
func matchScore(title, artist, album string, candidate Track) (float64, bool) {
	titleSimilarity := Similarity(title, Normalize(candidate.Title))
	if titleSimilarity < minTitleSimilarity {
		return 0, false
	}
	artistSimilarity := Similarity(artist, Normalize(candidate.ArtistName))
	if album != "" {
		albumSimilarity := Similarity(album, Normalize(candidate.AlbumTitle))
		return 0.55*titleSimilarity + 0.30*artistSimilarity + 0.15*albumSimilarity, true
	}
	return 0.65*titleSimilarity + 0.35*artistSimilarity, true
}

func albumKey(title, artist, album string) string {
	return title + "\x1f" + artist + "\x1f" + album
}

func artistKey(title, artist string) string {
	return title + "\x1f" + artist
}

func firstRune(s string) string {
	for _, r := range s {
		return string(r)
	}
	return ""
}
