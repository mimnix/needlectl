// Package api is a small typed client for the DroppedNeedle REST API
// (/api/v1). Only the endpoints needlectl needs are implemented.
package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	libraryPageSize = 200
	maxLibraryPages = 1000
	albumPageSize   = 50
	maxAlbumPages   = 40
	maxResponseSize = 32 << 20 // 32 MiB
	requestTimeout  = 5 * time.Minute
)

// Error is a non-2xx response from the DroppedNeedle API.
type Error struct {
	StatusCode int
	Method     string
	Path       string
	Detail     string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("%s %s: unexpected status %d", e.Method, e.Path, e.StatusCode)
	}
	return fmt.Sprintf("%s %s: %s (status %d)", e.Method, e.Path, e.Detail, e.StatusCode)
}

// IsNotFound reports whether the error is a 404 response.
func (e *Error) IsNotFound() bool { return e.StatusCode == http.StatusNotFound }

// IsUnauthorized reports whether the error is a 401/403 response.
func (e *Error) IsUnauthorized() bool {
	return e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden
}

// Client talks to a single DroppedNeedle instance.
type Client struct {
	baseURL   string
	http      *http.Client
	token     string
	userAgent string
}

// New creates a client for baseURL. Missing schemes default to http.
func New(baseURL string, insecureSkipVerify bool) (*Client, error) {
	normalized, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in for self-signed instances
	}

	return &Client{
		baseURL: normalized,
		http: &http.Client{
			Timeout:   requestTimeout,
			Transport: transport,
		},
		userAgent: "needlectl",
	}, nil
}

// BaseURL returns the normalized server URL.
func (c *Client) BaseURL() string { return c.baseURL }

// SetToken sets the bearer token used for authenticated requests.
func (c *Client) SetToken(token string) { c.token = token }

// Login authenticates with username/password and stores the returned token.
func (c *Client) Login(ctx context.Context, username, password string) (*User, error) {
	body := map[string]string{"username": username, "password": password}
	data, err := c.do(ctx, http.MethodPost, "/api/v1/auth/login", body)
	if err != nil {
		return nil, err
	}
	var resp AuthResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode login response: %w", err)
	}
	if resp.Token == "" {
		return nil, errors.New("login succeeded but no token was returned")
	}
	c.token = resp.Token
	return &resp.User, nil
}

// Me returns the authenticated user.
func (c *Client) Me(ctx context.Context) (*User, error) {
	data, err := c.do(ctx, http.MethodGet, "/api/v1/auth/me", nil)
	if err != nil {
		return nil, err
	}
	var user User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("decode user response: %w", err)
	}
	return &user, nil
}

// ListLibraryTracks returns every track of the native library.
func (c *Client) ListLibraryTracks(ctx context.Context) ([]LibraryTrack, error) {
	return c.listLibraryTracks(ctx, "")
}

// SearchLibraryTracks returns the library tracks matching query. An empty query
// lists every track.
func (c *Client) SearchLibraryTracks(ctx context.Context, query string) ([]LibraryTrack, error) {
	return c.listLibraryTracks(ctx, query)
}

// listLibraryTracks follows pagination until the reported total is reached.
func (c *Client) listLibraryTracks(ctx context.Context, search string) ([]LibraryTrack, error) {
	var all []LibraryTrack
	offset := 0
	total := 0
	complete := false

	for page := 0; page < maxLibraryPages; page++ {
		params := url.Values{}
		params.Set("limit", strconv.Itoa(libraryPageSize))
		params.Set("offset", strconv.Itoa(offset))
		if trimmed := strings.TrimSpace(search); trimmed != "" {
			params.Set("q", trimmed)
		}

		data, err := c.do(ctx, http.MethodGet, "/api/v1/library/tracks?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		var resp libraryTracksResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("decode library tracks: %w", err)
		}
		if page > 0 && resp.Offset != offset {
			return nil, fmt.Errorf("library listing returned offset %d for requested offset %d; refusing to work from an inconsistent library", resp.Offset, offset)
		}

		total = resp.Total
		all = append(all, resp.Items...)
		if len(resp.Items) == 0 {
			complete = true
			break
		}
		offset += len(resp.Items)
		if total > 0 && offset >= total {
			complete = true
			break
		}
	}

	if !complete {
		return nil, fmt.Errorf("library listing was truncated after %d pages (%d of %d tracks); refusing to work from an incomplete library", maxLibraryPages, len(all), total)
	}
	return all, nil
}

// ListLibraryAlbums returns the library albums matching query. An empty query
// lists every album. It follows pagination and fails rather than silently
// truncating when too many albums match.
func (c *Client) ListLibraryAlbums(ctx context.Context, query string) ([]LibraryAlbum, error) {
	var all []LibraryAlbum
	complete := false

	for page := 1; page <= maxAlbumPages; page++ {
		params := url.Values{}
		params.Set("page", strconv.Itoa(page))
		params.Set("page_size", strconv.Itoa(albumPageSize))
		if trimmed := strings.TrimSpace(query); trimmed != "" {
			params.Set("q", trimmed)
		}

		data, err := c.do(ctx, http.MethodGet, "/api/v1/library/albums?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		var resp libraryAlbumsResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("decode library albums: %w", err)
		}

		all = append(all, resp.Items...)
		if len(resp.Items) == 0 || len(all) >= resp.Total {
			complete = true
			break
		}
	}

	if !complete {
		return nil, fmt.Errorf("too many albums match %q; refine the query or use --id", query)
	}
	return all, nil
}

// GetLibraryAlbum returns one album of the library by its id.
func (c *Client) GetLibraryAlbum(ctx context.Context, albumID string) (*LibraryAlbum, error) {
	path := "/api/v1/library/albums/" + url.PathEscape(albumID)
	data, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var album LibraryAlbum
	if err := json.Unmarshal(data, &album); err != nil {
		return nil, fmt.Errorf("decode album: %w", err)
	}
	return &album, nil
}

// DeleteAlbum removes an album and cascades to its tracks. When deleteFiles is
// true the files are also removed from disk; when stopWanted is false the album
// stays queued for future automatic downloads.
func (c *Client) DeleteAlbum(ctx context.Context, albumID string, deleteFiles, stopWanted bool) (*CatalogRemovalResponse, error) {
	params := url.Values{}
	params.Set("delete_files", strconv.FormatBool(deleteFiles))
	params.Set("stop_wanted", strconv.FormatBool(stopWanted))

	path := "/api/v1/library/album/" + url.PathEscape(albumID) + "?" + params.Encode()
	data, err := c.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	var resp CatalogRemovalResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode album removal: %w", err)
	}
	return &resp, nil
}

// DeleteTrack removes a single track from the library.
//
// The DroppedNeedle endpoint has no catalog-only variant: the backing file is
// always deleted from disk.
func (c *Client) DeleteTrack(ctx context.Context, trackID string) (*CatalogRemovalResponse, error) {
	path := "/api/v1/library/tracks/" + url.PathEscape(trackID)
	data, err := c.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	var resp CatalogRemovalResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode track removal: %w", err)
	}
	return &resp, nil
}

// ListPlaylists returns the playlists visible to the authenticated user.
func (c *Client) ListPlaylists(ctx context.Context) ([]PlaylistSummary, error) {
	data, err := c.do(ctx, http.MethodGet, "/api/v1/playlists", nil)
	if err != nil {
		return nil, err
	}
	var resp playlistListResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode playlists: %w", err)
	}
	return resp.Playlists, nil
}

// CreatePlaylist creates a new playlist with the given name.
func (c *Client) CreatePlaylist(ctx context.Context, name string) (*PlaylistDetail, error) {
	data, err := c.do(ctx, http.MethodPost, "/api/v1/playlists", map[string]string{"name": name})
	if err != nil {
		return nil, err
	}
	var detail PlaylistDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		return nil, fmt.Errorf("decode created playlist: %w", err)
	}
	return &detail, nil
}

// GetPlaylist returns a playlist and its tracks.
func (c *Client) GetPlaylist(ctx context.Context, id string) (*PlaylistDetail, error) {
	data, err := c.do(ctx, http.MethodGet, "/api/v1/playlists/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	var detail PlaylistDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		return nil, fmt.Errorf("decode playlist: %w", err)
	}
	return &detail, nil
}

// AddTracks appends (or inserts at position) tracks into a playlist.
func (c *Client) AddTracks(ctx context.Context, playlistID string, tracks []TrackInput, position *int) ([]PlaylistTrack, error) {
	if len(tracks) == 0 {
		return nil, nil
	}
	body := addTracksRequest{Tracks: tracks, Position: position}
	path := "/api/v1/playlists/" + url.PathEscape(playlistID) + "/tracks"
	data, err := c.do(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	var resp addTracksResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode added tracks: %w", err)
	}
	return resp.Tracks, nil
}

// RemoveTracks removes the given track entries from a playlist.
func (c *Client) RemoveTracks(ctx context.Context, playlistID string, trackIDs []string) error {
	if len(trackIDs) == 0 {
		return nil
	}
	body := removeTracksRequest{TrackIDs: trackIDs}
	path := "/api/v1/playlists/" + url.PathEscape(playlistID) + "/tracks/remove"
	_, err := c.do(ctx, http.MethodPost, path, body)
	return err
}

// ResolveSources links tracks to the sources now available for their albums.
func (c *Client) ResolveSources(ctx context.Context, playlistID string) (map[string][]string, error) {
	path := "/api/v1/playlists/" + url.PathEscape(playlistID) + "/resolve-sources"
	data, err := c.do(ctx, http.MethodPost, path, map[string]string{})
	if err != nil {
		return nil, err
	}
	var resp resolveSourcesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode resolved sources: %w", err)
	}
	return resp.Sources, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("read response from %s: %w", path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &Error{
			StatusCode: resp.StatusCode,
			Method:     method,
			Path:       path,
			Detail:     extractDetail(data),
		}
	}
	return data, nil
}

// extractDetail pulls the FastAPI "detail" field out of an error body, falling
// back to the raw (trimmed) body.
func extractDetail(data []byte) string {
	var envelope struct {
		Detail json.RawMessage `json:"detail"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && len(envelope.Detail) > 0 {
		var message string
		if err := json.Unmarshal(envelope.Detail, &message); err == nil {
			return message
		}
		return string(envelope.Detail)
	}
	return strings.TrimSpace(string(data))
}

// normalizeBaseURL validates and canonicalizes a server URL. A missing scheme
// defaults to https, except for loopback hosts where http is assumed. Embedded
// credentials and non-http(s) schemes are rejected.
func normalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("server URL is empty")
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = defaultScheme(trimmed) + "://" + trimmed
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid server URL %q: %w", raw, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid server URL %q: unsupported scheme %q (use http or https)", raw, parsed.Scheme)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("invalid server URL %q: remove the embedded credentials", raw)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("invalid server URL %q: missing host", raw)
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

// defaultScheme picks https for remote hosts and http for local ones.
func defaultScheme(hostPort string) string {
	host := hostPort
	if slash := strings.IndexByte(host, '/'); slash >= 0 {
		host = host[:slash]
	}
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	}
	host = strings.Trim(strings.ToLower(host), "[]")

	switch host {
	case "localhost", "127.0.0.1", "::1":
		return "http"
	}
	if strings.HasSuffix(host, ".localhost") {
		return "http"
	}
	return "https"
}
