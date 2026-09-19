package api

import "encoding/json"

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
	// CoverAvailable reports whether DroppedNeedle holds artwork for the album.
	// It is the server's own signal and is not always exact (a rare false
	// negative), but it keeps the generated cover links from 404ing.
	CoverAvailable bool `json:"cover_available"`
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
	// Identity fields, only present on the album detail response. They explain
	// why metadata edits can be refused.
	IdentificationStatus        string  `json:"identification_status"`
	ReviewID                    *string `json:"review_id"`
	ManagementIdentityReadiness string  `json:"management_identity_readiness"`
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
	CoverURL         *string  `json:"cover_url"`
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
	CoverURL         *string  `json:"cover_url,omitempty"`
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

// TagField is one editable metadata field of the library management tag editor.
// CurrentValue is a string, a list of strings, a number, a bool or null
// depending on Cardinality.
type TagField struct {
	FieldName    string  `json:"field_name"`
	Scope        string  `json:"scope"`
	Cardinality  string  `json:"cardinality"`
	CurrentValue any     `json:"current_value"`
	OverrideID   *string `json:"override_id"`
	OverrideMode *string `json:"override_mode"`
}

// StringValue returns the current value when the field holds a plain string.
func (f TagField) StringValue() string {
	if value, ok := f.CurrentValue.(string); ok {
		return value
	}
	return ""
}

// TagEditor is the editable metadata of one library track. Fields scoped to the
// album (such as "album") change for every track of that album.
type TagEditor struct {
	LocalTrackID     string     `json:"local_track_id"`
	LocalAlbumID     string     `json:"local_album_id"`
	ProfileName      string     `json:"profile_name"`
	SettingsRevision string     `json:"settings_revision"`
	PolicyRevision   string     `json:"policy_revision"`
	AcceptedIdentity bool       `json:"accepted_identity"`
	IdentityReason   *string    `json:"identity_reason"`
	Fields           []TagField `json:"fields"`
}

// Field returns the editor field with the given name.
func (e TagEditor) Field(name string) (TagField, bool) {
	for _, field := range e.Fields {
		if field.FieldName == name {
			return field, true
		}
	}
	return TagField{}, false
}

// TagEdit modes accepted by the tag edit preview endpoint.
const (
	TagEditModeSaveOverride   = "save_override"
	TagEditModeWriteOnce      = "write_once"
	TagEditModeResetCanonical = "reset_canonical"
)

// TagEditField is one field change of a tag edit preview. Value is omitted when
// the field is being reset to its canonical value.
type TagEditField struct {
	FieldName string `json:"field_name"`
	Value     any    `json:"value,omitempty"`
}

// TagEditPreviewRequest is the body of POST /library/management/tag-edit-previews.
type TagEditPreviewRequest struct {
	LocalTrackID             string         `json:"local_track_id"`
	Mode                     string         `json:"mode"`
	ExpectedSettingsRevision string         `json:"expected_settings_revision"`
	ExpectedPolicyRevision   string         `json:"expected_policy_revision"`
	Fields                   []TagEditField `json:"fields"`
	IdempotencyKey           string         `json:"idempotency_key"`
}

// PreviewRef is returned when a management preview is created.
type PreviewRef struct {
	JobID        string `json:"job_id"`
	PreviewToken string `json:"preview_token"`
}

// ManagementPreview is the state of a staged management job. OperationRowRevision
// is echoed back verbatim on apply, so it is kept raw to avoid guessing its type.
type ManagementPreview struct {
	ID                   string          `json:"id"`
	JobID                string          `json:"job_id"`
	State                string          `json:"state"`
	Stale                bool            `json:"stale"`
	StaleReasons         []string        `json:"stale_reasons"`
	Expired              bool            `json:"expired"`
	ReadyForConfirmation bool            `json:"ready_for_confirmation"`
	OperationRowRevision json.RawMessage `json:"operation_row_revision"`
}

// ApplyPreviewRequest is the body of POST /library/management/previews/{id}/apply.
type ApplyPreviewRequest struct {
	PreviewToken                 string          `json:"preview_token"`
	ExpectedOperationRowRevision json.RawMessage `json:"expected_operation_row_revision"`
	IdempotencyKey               string          `json:"idempotency_key"`
	Confirmation                 bool            `json:"confirmation"`
}

// OperationRef identifies the queued operation an applied preview produced.
type OperationRef struct {
	ID string `json:"id"`
}

// Item statuses reported by the drop import endpoints.
const (
	ImportStatusImported    = "imported"
	ImportStatusNeedsReview = "needs_review"
	ImportStatusDiscarded   = "discarded"
)

// ImportItem is one group of uploaded files inside an import job. DroppedNeedle
// identifies an item as a unit, so matching always names an item rather than an
// individual file.
type ImportItem struct {
	ID               int     `json:"id"`
	FolderName       string  `json:"folder_name"`
	Status           string  `json:"status"`
	UpdatedAt        float64 `json:"updated_at"`
	ReleaseGroupMBID *string `json:"release_group_mbid"`
	AlbumTitle       *string `json:"album_title"`
	ArtistName       *string `json:"artist_name"`
	FilesTotal       int     `json:"files_total"`
	FilesImported    int     `json:"files_imported"`
	Detail           *string `json:"detail"`
}

// NeedsReview reports whether DroppedNeedle could not identify the item and is
// waiting for a manual match.
func (i ImportItem) NeedsReview() bool { return i.Status == ImportStatusNeedsReview }

// Settled reports whether the item reached a final state on its own and will not
// change again while it is polled.
func (i ImportItem) Settled() bool {
	switch i.Status {
	case ImportStatusImported, ImportStatusNeedsReview, ImportStatusDiscarded:
		return true
	}
	return false
}

// Title returns the album title of the item, or an empty string when unknown.
func (i ImportItem) Title() string { return derefString(i.AlbumTitle) }

// Artist returns the album artist of the item, or an empty string when unknown.
func (i ImportItem) Artist() string { return derefString(i.ArtistName) }

// Message returns the server detail for the item, or an empty string.
func (i ImportItem) Message() string { return derefString(i.Detail) }

// MatchedReleaseGroup returns the release group the item was matched to, or an
// empty string when it is still unbound.
func (i ImportItem) MatchedReleaseGroup() string { return derefString(i.ReleaseGroupMBID) }

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// ImportJob is one upload batch: the files sent in a single request and the
// items DroppedNeedle derived from them.
type ImportJob struct {
	ID         string       `json:"id"`
	Status     string       `json:"status"`
	CreatedAt  float64      `json:"created_at"`
	UploadName string       `json:"upload_name"`
	UserID     string       `json:"user_id"`
	UserName   string       `json:"user_name"`
	Error      *string      `json:"error"`
	Items      []ImportItem `json:"items"`
}

// Settled reports whether every item of the job reached a final state.
func (j ImportJob) Settled() bool {
	for _, item := range j.Items {
		if !item.Settled() {
			return false
		}
	}
	return true
}

// ErrorMessage returns the job level error, or an empty string when there is none.
func (j ImportJob) ErrorMessage() string { return derefString(j.Error) }

type importJobsResponse struct {
	Jobs []ImportJob `json:"jobs"`
}
