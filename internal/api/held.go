package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// HeldEntry is one download or upload DroppedNeedle kept out of the library
// until a person decides what it is. It is the "unmatched" list: the files are
// on disk, but no album was accepted for them.
type HeldEntry struct {
	ID               int64      `json:"id"`
	Origin           string     `json:"origin"`
	State            string     `json:"state"`
	AlbumTitle       *string    `json:"album_title"`
	ArtistName       *string    `json:"artist_name"`
	ReleaseGroupMBID *string    `json:"release_group_mbid"`
	RecordingMBID    *string    `json:"recording_mbid"`
	DropItemID       *int64     `json:"drop_item_id"`
	CheckAlbumID     *string    `json:"check_album_id"`
	CreatedAt        int64      `json:"created_at"`
	ExpiresAt        *int64     `json:"expires_at"`
	NextRetryAt      *int64     `json:"next_retry_at"`
	Files            []HeldFile `json:"files"`
}

// HeldFile is one file of a held entry.
type HeldFile struct {
	ID                   int64    `json:"id"`
	OriginalName         string   `json:"original_name"`
	ReleaseMBID          *string  `json:"release_mbid"`
	ReleaseTrackMBID     *string  `json:"release_track_mbid"`
	RecordingMBID        *string  `json:"recording_mbid"`
	DiscNumber           *int     `json:"disc_number"`
	TrackNumber          *int     `json:"track_number"`
	TrackTitle           *string  `json:"track_title"`
	Format               *string  `json:"file_format"`
	DurationSeconds      *float64 `json:"duration_seconds"`
	ExpectedDurationSecs *float64 `json:"expected_duration_seconds"`
	EvidenceTitle        *string  `json:"evidence_title"`
	EvidenceArtist       *string  `json:"evidence_artist"`
	EvidenceScore        *float64 `json:"evidence_score"`
	Source               string   `json:"source"`
	SourceTaskID         *string  `json:"source_task_id"`
	Reason               string   `json:"reason"`
	ReasonText           string   `json:"reason_text"`
	ReasonAction         string   `json:"reason_action"`
	ReasonKind           string   `json:"reason_kind"`
	ReasonDetail         *string  `json:"reason_detail"`
	Actions              []string `json:"actions"`
	CreatedAt            int64    `json:"created_at"`
}

// Title returns the entry's album title, or an empty string when it has none.
func (e HeldEntry) Title() string { return derefString(e.AlbumTitle) }

// Artist returns the entry's artist, or an empty string when it has none.
func (e HeldEntry) Artist() string { return derefString(e.ArtistName) }

// ReleaseGroup returns the entry's current release-group guess, or an empty
// string when it has none.
func (e HeldEntry) ReleaseGroup() string { return derefString(e.ReleaseGroupMBID) }

// ReasonCode returns the reason the entry was held: the first file's reason,
// which is the one DroppedNeedle reports for the entry as a whole.
func (e HeldEntry) ReasonCode() string {
	if len(e.Files) == 0 {
		return ""
	}
	return e.Files[0].Reason
}

// HeldActionResult reports what a match or import did with an entry's files.
type HeldActionResult struct {
	Imported     int     `json:"imported"`
	AlreadyOwned int     `json:"already_owned"`
	StillHeld    int     `json:"still_held"`
	Discarded    int     `json:"discarded"`
	Message      *string `json:"message"`
}

type heldListResponse struct {
	Entries []HeldEntry `json:"entries"`
}

// heldMatchRequest is the body of POST /held/{id}/match. DroppedNeedle links
// held files to an album, so the reference is a release group, not a release.
type heldMatchRequest struct {
	ReleaseGroupMBID string  `json:"release_group_mbid"`
	FileIDs          []int64 `json:"file_ids,omitempty"`
}

// ListHeldEntries returns the entries waiting for a decision, newest first.
// Admins can pass all to read every user's entries.
func (c *Client) ListHeldEntries(ctx context.Context, all bool) ([]HeldEntry, error) {
	path := "/api/v3/held"
	if all {
		path += "?all=true"
	}
	data, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var resp heldListResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode held entries: %w", err)
	}
	return resp.Entries, nil
}

// MatchHeldEntry imports every file of a held entry as the album named by
// releaseGroupMBID, pairing the files with that album's tracks.
//
// This is the manual decision DroppedNeedle asks for when identification could
// not accept an album: unlike the automatic checks, it is not refused because
// the files' tags disagree with the release.
func (c *Client) MatchHeldEntry(ctx context.Context, entryID int64, releaseGroupMBID string) (*HeldActionResult, error) {
	if entryID <= 0 {
		return nil, fmt.Errorf("invalid held entry id %d", entryID)
	}
	trimmed := strings.TrimSpace(releaseGroupMBID)
	if trimmed == "" {
		return nil, errors.New("release group mbid is empty")
	}

	path := "/api/v3/held/" + strconv.FormatInt(entryID, 10) + "/match"
	body := heldMatchRequest{ReleaseGroupMBID: trimmed}
	data, err := c.do(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}

	var result HeldActionResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode held match result: %w", err)
	}
	return &result, nil
}
