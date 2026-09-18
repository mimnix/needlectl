package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTrackInputMarshalsCoverURLOnlyWhenSet(t *testing.T) {
	cover := "/api/v1/covers/release-group/rg-1?size=250"

	withCover, err := json.Marshal(TrackInput{TrackName: "Disorder", CoverURL: &cover})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if !strings.Contains(string(withCover), `"cover_url":"/api/v1/covers/release-group/rg-1?size=250"`) {
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
	raw := `{"id":"e1","track_name":"Disorder","cover_url":"/api/v1/covers/release-group/rg-1?size=250"}`

	var track PlaylistTrack
	if err := json.Unmarshal([]byte(raw), &track); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if track.CoverURL == nil || *track.CoverURL != "/api/v1/covers/release-group/rg-1?size=250" {
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

func TestLibraryTrackReadsCoverAvailable(t *testing.T) {
	var track LibraryTrack
	if err := json.Unmarshal([]byte(`{"id":"f1","cover_available":true}`), &track); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if !track.CoverAvailable {
		t.Error("CoverAvailable = false, want true")
	}
}

func TestLibraryAlbumReadsIdentityFields(t *testing.T) {
	raw := `{"id":"a1","album_identity_state":"local_only",
	         "identification_status":"needs_review","review_id":"r-1",
	         "management_identity_readiness":"exact_release_required"}`

	var album LibraryAlbum
	if err := json.Unmarshal([]byte(raw), &album); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if album.AlbumIdentityState != "local_only" || album.IdentificationStatus != "needs_review" {
		t.Errorf("album = %+v", album)
	}
	if album.ReviewID == nil || *album.ReviewID != "r-1" {
		t.Errorf("ReviewID = %v", album.ReviewID)
	}
	if album.ManagementIdentityReadiness != "exact_release_required" {
		t.Errorf("ManagementIdentityReadiness = %q", album.ManagementIdentityReadiness)
	}

	var withoutReview LibraryAlbum
	if err := json.Unmarshal([]byte(`{"id":"a2","review_id":null}`), &withoutReview); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if withoutReview.ReviewID != nil {
		t.Errorf("ReviewID = %v, want nil", *withoutReview.ReviewID)
	}
}
