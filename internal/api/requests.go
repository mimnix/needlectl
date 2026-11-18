package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// SearchResultItem is one hit of the server's MusicBrainz-backed search. Only
// the fields needlectl reads are decoded.
type SearchResultItem struct {
	Kind          string  `json:"kind"`
	Title         string  `json:"title"`
	Artist        *string `json:"artist"`
	MusicBrainzID *string `json:"musicbrainz_id"`
	Score         int     `json:"score"`
	InLibrary     bool    `json:"in_library"`
	Requested     bool    `json:"requested"`
	Year          *int    `json:"year"`
	TypeInfo      *string `json:"type_info"`
}

type searchBucketResponse struct {
	Results []SearchResultItem `json:"results"`
}

// SearchAlbums searches the server's album index (library rows and MusicBrainz
// releases) and returns the hits best first.
func (c *Client) SearchAlbums(ctx context.Context, query string, limit int) ([]SearchResultItem, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, errors.New("album search query is empty")
	}
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	params := url.Values{}
	params.Set("q", trimmed)
	params.Set("limit", strconv.Itoa(limit))

	data, err := c.do(ctx, http.MethodGet, "/api/v3/search/albums?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var resp searchBucketResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode album search: %w", err)
	}
	return resp.Results, nil
}

// AlbumRequest asks DroppedNeedle to acquire one release group. Curators get the
// download dispatched at once; other users wait for approval.
type AlbumRequest struct {
	MusicBrainzID string  `json:"musicbrainz_id"`
	Album         *string `json:"album,omitempty"`
	Artist        *string `json:"artist,omitempty"`
	Year          *int    `json:"year,omitempty"`
}

// IntakeResponse is the outcome of an album (or track) request.
type IntakeResponse struct {
	Success       bool    `json:"success"`
	Message       string  `json:"message"`
	MusicBrainzID string  `json:"musicbrainz_id"`
	Status        string  `json:"status"`
	TaskID        *string `json:"task_id"`
}

// RequestAlbum asks for one release group by its MusicBrainz id.
func (c *Client) RequestAlbum(ctx context.Context, request AlbumRequest) (*IntakeResponse, error) {
	trimmed := strings.TrimSpace(request.MusicBrainzID)
	if trimmed == "" {
		return nil, errors.New("release group mbid is empty")
	}
	request.MusicBrainzID = trimmed

	data, err := c.do(ctx, http.MethodPost, "/api/v3/requests/albums", request)
	if err != nil {
		return nil, err
	}
	var resp IntakeResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode album request: %w", err)
	}
	return &resp, nil
}

// TrackRequest asks DroppedNeedle to acquire one exact recording.
type TrackRequest struct {
	RecordingMBID    string  `json:"recording_mbid"`
	ArtistName       string  `json:"artist_name"`
	TrackTitle       string  `json:"track_title"`
	AlbumTitle       *string `json:"album_title,omitempty"`
	ArtistMBID       *string `json:"artist_mbid,omitempty"`
	ReleaseGroupMBID *string `json:"release_group_mbid,omitempty"`
	ReleaseMBID      *string `json:"release_mbid,omitempty"`
	DurationSeconds  *int    `json:"duration_seconds,omitempty"`
}

// TrackIntakeResponse is the outcome of a track request: queued, or
// awaiting_approval for a non-curator.
type TrackIntakeResponse struct {
	Status string  `json:"status"`
	TaskID *string `json:"task_id"`
}

// RequestTrack asks for one recording by its MusicBrainz id.
func (c *Client) RequestTrack(ctx context.Context, request TrackRequest) (*TrackIntakeResponse, error) {
	request.RecordingMBID = strings.TrimSpace(request.RecordingMBID)
	request.ArtistName = strings.TrimSpace(request.ArtistName)
	request.TrackTitle = strings.TrimSpace(request.TrackTitle)
	switch {
	case request.RecordingMBID == "":
		return nil, errors.New("recording mbid is empty")
	case request.ArtistName == "":
		return nil, errors.New("track artist is empty")
	case request.TrackTitle == "":
		return nil, errors.New("track title is empty")
	}

	data, err := c.do(ctx, http.MethodPost, "/api/v3/requests/tracks", request)
	if err != nil {
		return nil, err
	}
	var resp TrackIntakeResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode track request: %w", err)
	}
	return &resp, nil
}
