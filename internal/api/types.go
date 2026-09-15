package api

// User is the authenticated account returned by the auth endpoints.
type User struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

// AuthResponse is returned by POST /api/v1/auth/login.
type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// LibraryTrack is one entry of the native DroppedNeedle library.
type LibraryTrack struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	AlbumID         string  `json:"album_id"`
	AlbumTitle      string  `json:"album_title"`
	ArtistID        string  `json:"artist_id"`
	ArtistName      string  `json:"artist_name"`
	DiscNumber      int     `json:"disc_number"`
	TrackNumber     int     `json:"track_number"`
	DurationSeconds float64 `json:"duration_seconds"`
	Format          string  `json:"format"`
}

type libraryTracksResponse struct {
	Items  []LibraryTrack `json:"items"`
	Total  int            `json:"total"`
	Offset int            `json:"offset"`
	Limit  int            `json:"limit"`
}

// LibraryAlbum is one album of the native DroppedNeedle library.
type LibraryAlbum struct {
	ID                        string  `json:"id"`
	Title                     string  `json:"title"`
	ArtistID                  string  `json:"artist_id"`
	ArtistName                string  `json:"artist_name"`
	MusicBrainzReleaseGroupID *string `json:"musicbrainz_release_group_id"`
	AlbumIdentityState        string  `json:"album_identity_state"`
	TrackCount                int     `json:"track_count"`
	TotalDurationSeconds      float64 `json:"total_duration_seconds"`
	TotalSizeBytes            int64   `json:"total_size_bytes"`
	Format                    *string `json:"format"`
	Year                      *int    `json:"year"`
}

type libraryAlbumsResponse struct {
	Items []LibraryAlbum `json:"items"`
	Total int            `json:"total"`
}

// CatalogRemovalResponse is returned by the album/track removal endpoints. The
// server cascades album deletion, so RemovedTrackIDs lists every track that was
// removed with it.
type CatalogRemovalResponse struct {
	Success         bool     `json:"success"`
	ID              string   `json:"id"`
	RemovedTrackIDs []string `json:"removed_track_ids"`
}

// PlaylistSummary is one row of GET /api/v1/playlists.
type PlaylistSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	TrackCount int    `json:"track_count"`
}

type playlistListResponse struct {
	Playlists []PlaylistSummary `json:"playlists"`
}

// PlaylistTrack is one entry of a playlist.
type PlaylistTrack struct {
	ID               string   `json:"id"`
	Position         int      `json:"position"`
	TrackName        string   `json:"track_name"`
	ArtistName       string   `json:"artist_name"`
	AlbumName        string   `json:"album_name"`
	AlbumID          *string  `json:"album_id"`
	ArtistID         *string  `json:"artist_id"`
	TrackSourceID    *string  `json:"track_source_id"`
	LibraryFileID    *string  `json:"library_file_id"`
	SourceType       string   `json:"source_type"`
	AvailableSources []string `json:"available_sources"`
	TrackNumber      *int     `json:"track_number"`
	DiscNumber       *int     `json:"disc_number"`
	Duration         *int     `json:"duration"`
}

// Linked reports whether the track entry is actually tied to a playable
// source. "available_sources" lists candidate sources and is deliberately NOT
// enough: an entry can advertise candidates while still being unlinked.
func (t PlaylistTrack) Linked() bool {
	if t.LibraryFileID != nil && *t.LibraryFileID != "" {
		return true
	}
	if t.TrackSourceID != nil && *t.TrackSourceID != "" {
		return true
	}
	return t.SourceType != ""
}

// PlaylistDetail is the full playlist document, including its tracks.
type PlaylistDetail struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Tracks     []PlaylistTrack `json:"tracks"`
	TrackCount int             `json:"track_count"`
}

// TrackInput is one element of the AddTracks request body.
type TrackInput struct {
	TrackName        string   `json:"track_name"`
	ArtistName       string   `json:"artist_name"`
	AlbumName        string   `json:"album_name"`
	AlbumID          *string  `json:"album_id,omitempty"`
	ArtistID         *string  `json:"artist_id,omitempty"`
	TrackSourceID    *string  `json:"track_source_id,omitempty"`
	SourceType       string   `json:"source_type"`
	AvailableSources []string `json:"available_sources,omitempty"`
	Format           *string  `json:"format,omitempty"`
	TrackNumber      *int     `json:"track_number,omitempty"`
	DiscNumber       *int     `json:"disc_number,omitempty"`
	Duration         *int     `json:"duration,omitempty"`
}

type addTracksRequest struct {
	Tracks   []TrackInput `json:"tracks"`
	Position *int         `json:"position,omitempty"`
}

type addTracksResponse struct {
	Tracks []PlaylistTrack `json:"tracks"`
}

type removeTracksRequest struct {
	TrackIDs []string `json:"track_ids"`
}

type resolveSourcesResponse struct {
	Sources map[string][]string `json:"sources"`
}
