package mbref

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReleaseGroupTracksReadsTheOfficialRelease(t *testing.T) {
	var requested []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		switch {
		case strings.HasPrefix(r.URL.Path, "/ws/2/release-group/"):
			writeMBJSON(t, w, map[string]any{
				"id": "9b191575-fa1f-3531-bff5-121782318972",
				"releases": []any{
					map[string]any{"id": "11111111-1111-1111-1111-111111111111", "date": "1990", "status": "Bootleg"},
					map[string]any{"id": "22222222-2222-2222-2222-222222222222", "date": "2011", "status": "Official"},
					map[string]any{"id": "33333333-3333-3333-3333-333333333333", "date": "1973", "status": "Official"},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/ws/2/release/33333333-3333-3333-3333-333333333333"):
			writeMBJSON(t, w, map[string]any{
				"id": "33333333-3333-3333-3333-333333333333",
				"media": []any{
					map[string]any{
						"position": 1,
						"tracks": []any{
							map[string]any{
								"position":  2,
								"title":     "The Real Me",
								"length":    201906,
								"recording": map[string]any{"id": "A87F7F4A-16EF-4068-AC61-B365756B610E"},
								"artist-credit": []any{
									map[string]any{"artist": map[string]any{"name": "The Who"}},
								},
							},
						},
					},
				},
			})
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := &Resolver{BaseURL: server.URL}
	tracks, err := resolver.ReleaseGroupTracks(context.Background(), "9b191575-fa1f-3531-bff5-121782318972")
	if err != nil {
		t.Fatalf("ReleaseGroupTracks returned error: %v", err)
	}
	if len(tracks) != 1 {
		t.Fatalf("tracks = %d, want 1", len(tracks))
	}
	track := tracks[0]
	if track.RecordingMBID != "a87f7f4a-16ef-4068-ac61-b365756b610e" {
		t.Errorf("recording = %q, want the lowercased id", track.RecordingMBID)
	}
	if track.Title != "The Real Me" || track.Artist != "The Who" {
		t.Errorf("track = %+v", track)
	}
	if track.DiscNumber != 1 || track.TrackNumber != 2 {
		t.Errorf("disc/track = %d/%d, want 1/2", track.DiscNumber, track.TrackNumber)
	}
	if seconds := track.Seconds(); seconds == nil || *seconds != 202 {
		t.Errorf("Seconds = %v, want 202", seconds)
	}

	// The bootleg is skipped and the earliest official release is read first.
	if len(requested) != 2 || !strings.Contains(requested[1], "33333333-3333-3333-3333-333333333333") {
		t.Errorf("requests = %v, want the release-group lookup then 33333333-3333-3333-3333-333333333333", requested)
	}
}

func TestReleaseGroupTracksTriesAnotherRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/ws/2/release-group/"):
			writeMBJSON(t, w, map[string]any{
				"releases": []any{
					map[string]any{"id": "44444444-4444-4444-4444-444444444444", "date": "1973", "status": "Official"},
					map[string]any{"id": "55555555-5555-5555-5555-555555555555", "date": "1990", "status": "Official"},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/ws/2/release/44444444-4444-4444-4444-444444444444"):
			writeMBJSON(t, w, map[string]any{"media": []any{}})
		case strings.HasPrefix(r.URL.Path, "/ws/2/release/55555555-5555-5555-5555-555555555555"):
			writeMBJSON(t, w, map[string]any{
				"media": []any{
					map[string]any{
						"position": 1,
						"tracks": []any{
							map[string]any{
								"position":  1,
								"title":     "Moonglow",
								"length":    236000,
								"recording": map[string]any{"id": "28c891ed-f8ee-476a-aefd-b5e704340583"},
								"artist-credit": []any{
									map[string]any{"artist": map[string]any{"name": "Avantasia"}},
								},
							},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := &Resolver{BaseURL: server.URL}
	tracks, err := resolver.ReleaseGroupTracks(context.Background(), "9b191575-fa1f-3531-bff5-121782318972")
	if err != nil {
		t.Fatalf("ReleaseGroupTracks returned error: %v", err)
	}
	if len(tracks) != 1 || tracks[0].Title != "Moonglow" {
		t.Errorf("tracks = %+v, want Moonglow from the second release", tracks)
	}
}

func TestReleaseGroupTracksRejectsAnInvalidID(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer server.Close()

	resolver := &Resolver{BaseURL: server.URL}
	if _, err := resolver.ReleaseGroupTracks(context.Background(), "not-an-mbid"); err == nil {
		t.Error("ReleaseGroupTracks accepted an invalid id")
	}
	if requests != 0 {
		t.Errorf("client sent %d requests, want none", requests)
	}
}

func TestReleaseTrackSecondsRoundTheLength(t *testing.T) {
	if got := (ReleaseTrack{DurationMS: 0}).Seconds(); got != nil {
		t.Errorf("Seconds(0) = %v, want nil", *got)
	}
	if got := (ReleaseTrack{DurationMS: 236000}).Seconds(); got == nil || *got != 236 {
		t.Errorf("Seconds(236000) = %v, want 236", got)
	}
}

func writeMBJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}
