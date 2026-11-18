package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTrackInputMarshalsCoverURLOnlyWhenSet(t *testing.T) {
	cover := "/api/v3/covers/release-group/rg-1?size=250"

	withCover, err := json.Marshal(TrackInput{TrackName: "Disorder", CoverURL: &cover})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if !strings.Contains(string(withCover), `"cover_url":"/api/v3/covers/release-group/rg-1?size=250"`) {
		t.Errorf("payload %s does not carry cover_url", withCover)
	}

	withoutCover, err := json.Marshal(TrackInput{TrackName: "Disorder"})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(withoutCover), "cover_url") {
		t.Errorf("payload %s should omit cover_url", withoutCover)
	}
}

func TestPlaylistTrackReadsCoverURL(t *testing.T) {
	raw := `{"id":"e1","track_name":"Disorder","cover_url":"/api/v3/covers/release-group/rg-1?size=250"}`

	var track PlaylistTrack
	if err := json.Unmarshal([]byte(raw), &track); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if track.CoverURL == nil || *track.CoverURL != "/api/v3/covers/release-group/rg-1?size=250" {
		t.Errorf("CoverURL = %v, want the release-group link", track.CoverURL)
	}
}

func TestPlaylistTrackToleratesAMissingCoverURL(t *testing.T) {
	var track PlaylistTrack
	if err := json.Unmarshal([]byte(`{"id":"e1"}`), &track); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if track.CoverURL != nil {
		t.Errorf("CoverURL = %v, want nil", *track.CoverURL)
	}
}

func TestPlaylistTrackReadsAFractionalDuration(t *testing.T) {
	var track PlaylistTrack
	if err := json.Unmarshal([]byte(`{"id":"e1","duration":215.4}`), &track); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if track.Duration == nil || *track.Duration != 215.4 {
		t.Errorf("Duration = %v, want 215.4", track.Duration)
	}
}

func TestLibraryTrackReadsCoverAvailable(t *testing.T) {
	var track LibraryTrack
	if err := json.Unmarshal([]byte(`{"id":"f1","cover_available":true}`), &track); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if !track.CoverAvailable {
		t.Error("CoverAvailable = false, want true")
	}
}

func TestLibraryTrackReadsTheReleaseGroup(t *testing.T) {
	var track LibraryTrack
	raw := `{"id":"f1","album_id":"local-1","release_group_mbid":"rg-1"}`
	if err := json.Unmarshal([]byte(raw), &track); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if track.AlbumID != "local-1" {
		t.Errorf("AlbumID = %q, want the local album id", track.AlbumID)
	}
	if track.ReleaseGroupMBID == nil || *track.ReleaseGroupMBID != "rg-1" {
		t.Errorf("ReleaseGroupMBID = %v, want rg-1", track.ReleaseGroupMBID)
	}

	var without LibraryTrack
	if err := json.Unmarshal([]byte(`{"id":"f2","release_group_mbid":null}`), &without); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if without.ReleaseGroupMBID != nil {
		t.Errorf("ReleaseGroupMBID = %v, want nil", *without.ReleaseGroupMBID)
	}
}

func TestPlaylistTrackReadsFormatAndMBIDs(t *testing.T) {
	raw := `{"id":"e1","format":"flac","recording_mbid":"rec-1","plex_rating_key":"plex-1"}`

	var track PlaylistTrack
	if err := json.Unmarshal([]byte(raw), &track); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if track.Format == nil || *track.Format != "flac" {
		t.Errorf("Format = %v, want flac", track.Format)
	}
	if track.RecordingMBID == nil || *track.RecordingMBID != "rec-1" {
		t.Errorf("RecordingMBID = %v, want rec-1", track.RecordingMBID)
	}
	if track.PlexRatingKey == nil || *track.PlexRatingKey != "plex-1" {
		t.Errorf("PlexRatingKey = %v, want plex-1", track.PlexRatingKey)
	}
}

func TestLibraryTrackReadsRowRevision(t *testing.T) {
	var track LibraryTrack
	if err := json.Unmarshal([]byte(`{"id":"f1","row_revision":17}`), &track); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if track.RowRevision != 17 {
		t.Errorf("RowRevision = %d, want 17", track.RowRevision)
	}
}

func TestLibraryAlbumReadsIdentityFields(t *testing.T) {
	raw := `{"id":"a1","release_group_mbid":"rg-1","release_mbid":"rel-1",
	         "identity_state":"local_only","pending_review_id":"r-1",
	         "row_revision":11,"listing_revision":5,
	         "management_hold":{"kind":"handing_back","handing_back":true}}`

	var album LibraryAlbum
	if err := json.Unmarshal([]byte(raw), &album); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if album.IdentityState != "local_only" {
		t.Errorf("IdentityState = %q, want local_only", album.IdentityState)
	}
	if album.ReleaseGroupMBID == nil || *album.ReleaseGroupMBID != "rg-1" {
		t.Errorf("ReleaseGroupMBID = %v", album.ReleaseGroupMBID)
	}
	if album.ReleaseMBID == nil || *album.ReleaseMBID != "rel-1" {
		t.Errorf("ReleaseMBID = %v", album.ReleaseMBID)
	}
	if album.PendingReviewID == nil || *album.PendingReviewID != "r-1" {
		t.Errorf("PendingReviewID = %v", album.PendingReviewID)
	}
	if album.RowRevision != 11 || album.ListingRevision != 5 {
		t.Errorf("revisions = %d/%d, want 11/5", album.RowRevision, album.ListingRevision)
	}
	if album.ManagementHold == nil || album.ManagementHold.Kind != "handing_back" || !album.ManagementHold.HandingBack {
		t.Errorf("ManagementHold = %+v", album.ManagementHold)
	}

	var withoutReview LibraryAlbum
	if err := json.Unmarshal([]byte(`{"id":"a2","pending_review_id":null,"management_hold":null}`), &withoutReview); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if withoutReview.PendingReviewID != nil {
		t.Errorf("PendingReviewID = %v, want nil", *withoutReview.PendingReviewID)
	}
	if withoutReview.ManagementHold != nil {
		t.Errorf("ManagementHold = %+v, want nil", withoutReview.ManagementHold)
	}
}
