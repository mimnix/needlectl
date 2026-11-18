// Package api is a small typed client for the DroppedNeedle REST API
// (/api/v3). Only the endpoints needlectl needs are implemented.
package api

import (
	"bytes"
	"context"
	"crypto/rand"
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
//
// DroppedNeedle defaults to cookie sessions, which return no token. A CLI has no
// cookie jar, so bearer transport is requested explicitly.
func (c *Client) Login(ctx context.Context, username, password string) (*User, error) {
	body := map[string]string{"username": username, "password": password, "transport": "bearer"}
	data, err := c.do(ctx, http.MethodPost, "/api/v3/auth/login", body)
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
	data, err := c.do(ctx, http.MethodGet, "/api/v3/me", nil)
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

// listLibraryTracks follows the cursor pagination until the server reports no
// next page.
func (c *Client) listLibraryTracks(ctx context.Context, search string) ([]LibraryTrack, error) {
	var all []LibraryTrack
	cursor := ""
	previous := ""
	total := 0
	complete := false

	for page := 0; page < maxLibraryPages; page++ {
		params := url.Values{}
		params.Set("limit", strconv.Itoa(libraryPageSize))
		if cursor != "" {
			params.Set("after", cursor)
		}
		if trimmed := strings.TrimSpace(search); trimmed != "" {
			params.Set("q", trimmed)
		}

		data, err := c.do(ctx, http.MethodGet, "/api/v3/library/tracks?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		var resp libraryTracksResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("decode library tracks: %w", err)
		}

		total = resp.Total
		all = append(all, resp.Items...)
		next := ""
		if resp.Next != nil {
			next = strings.TrimSpace(*resp.Next)
		}
		if next == "" {
			complete = true
			break
		}
		if next == cursor || next == previous {
			return nil, errors.New("library listing returned a repeated page cursor; refusing to work from an inconsistent library")
		}
		if len(resp.Items) == 0 {
			return nil, errors.New("library listing returned a page cursor but no items; refusing to work from an inconsistent library")
		}
		previous, cursor = cursor, next
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
	cursor := ""
	previous := ""
	complete := false

	for page := 0; page < maxAlbumPages; page++ {
		params := url.Values{}
		params.Set("limit", strconv.Itoa(albumPageSize))
		if cursor != "" {
			params.Set("after", cursor)
		}
		if trimmed := strings.TrimSpace(query); trimmed != "" {
			params.Set("q", trimmed)
		}

		data, err := c.do(ctx, http.MethodGet, "/api/v3/library/albums?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		var resp libraryAlbumsResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("decode library albums: %w", err)
		}

		all = append(all, resp.Items...)
		next := ""
		if resp.Next != nil {
			next = strings.TrimSpace(*resp.Next)
		}
		if next == "" {
			complete = true
			break
		}
		if next == cursor || next == previous {
			return nil, errors.New("album listing returned a repeated page cursor; refusing to work from an inconsistent library")
		}
		if len(resp.Items) == 0 {
			return nil, errors.New("album listing returned a page cursor but no items; refusing to work from an inconsistent library")
		}
		previous, cursor = cursor, next
	}

	if !complete {
		return nil, fmt.Errorf("too many albums match %q; refine the query or use --id", query)
	}
	return all, nil
}

// GetLibraryAlbum returns one album of the library by its id.
func (c *Client) GetLibraryAlbum(ctx context.Context, albumID string) (*LibraryAlbum, error) {
	path := "/api/v3/library/albums/" + url.PathEscape(albumID)
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
//
// The endpoint demands both the row and the listing revision, and the list view
// does not carry them, so the album detail is loaded first.
func (c *Client) DeleteAlbum(ctx context.Context, albumID string, deleteFiles, stopWanted bool) (*RemovalResponse, error) {
	album, err := c.GetLibraryAlbum(ctx, albumID)
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	params.Set("expected_revision", strconv.FormatInt(album.RowRevision, 10))
	params.Set("expected_listing_revision", strconv.FormatInt(album.ListingRevision, 10))
	params.Set("delete_files", strconv.FormatBool(deleteFiles))
	params.Set("stop_wanted", strconv.FormatBool(stopWanted))

	path := "/api/v3/library/albums/" + url.PathEscape(albumID) + "?" + params.Encode()
	data, err := c.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	var resp RemovalResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode album removal: %w", err)
	}
	return &resp, nil
}

// DeleteTrack removes a single track from the library.
//
// The file is always deleted from disk: DroppedNeedle has no catalog-only
// variant for a single track. The endpoint requires the track's row revision,
// which is passed in when the caller already has it; otherwise it is looked up.
func (c *Client) DeleteTrack(ctx context.Context, trackID string, expectedRevision int64) (*RemovalResponse, error) {
	if expectedRevision <= 0 {
		revision, err := c.libraryTrackRevision(ctx, trackID)
		if err != nil {
			return nil, err
		}
		expectedRevision = revision
	}

	params := url.Values{}
	params.Set("expected_revision", strconv.FormatInt(expectedRevision, 10))
	params.Set("delete_file", "true")

	path := "/api/v3/library/tracks/" + url.PathEscape(trackID) + "?" + params.Encode()
	data, err := c.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	var resp RemovalResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("decode track removal: %w", err)
	}
	return &resp, nil
}

// libraryTrackRevision finds the row revision of one track. DroppedNeedle v3 has
// no single-track read endpoint, so the library listing is scanned.
func (c *Client) libraryTrackRevision(ctx context.Context, trackID string) (int64, error) {
	tracks, err := c.ListLibraryTracks(ctx)
	if err != nil {
		return 0, err
	}
	for _, track := range tracks {
		if track.ID == trackID {
			if track.RowRevision <= 0 {
				return 0, fmt.Errorf("track %s has no revision to delete with", trackID)
			}
			return track.RowRevision, nil
		}
	}
	return 0, fmt.Errorf("track %s is not in the library", trackID)
}

// ListPlaylists returns the playlists visible to the authenticated user.
func (c *Client) ListPlaylists(ctx context.Context) ([]PlaylistSummary, error) {
	data, err := c.do(ctx, http.MethodGet, "/api/v3/playlists", nil)
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
	data, err := c.do(ctx, http.MethodPost, "/api/v3/playlists", map[string]string{"name": name})
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
	data, err := c.do(ctx, http.MethodGet, "/api/v3/playlists/"+url.PathEscape(id), nil)
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
	path := "/api/v3/playlists/" + url.PathEscape(playlistID) + "/tracks"
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
	path := "/api/v3/playlists/" + url.PathEscape(playlistID) + "/tracks/remove"
	_, err := c.do(ctx, http.MethodPost, path, body)
	return err
}

// ResolveSources links tracks to the sources now available for their albums.
func (c *Client) ResolveSources(ctx context.Context, playlistID string) (map[string][]string, error) {
	path := "/api/v3/playlists/" + url.PathEscape(playlistID) + "/resolve-sources"
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

// ListAlbumTracks returns the library tracks of one album.
func (c *Client) ListAlbumTracks(ctx context.Context, albumID string) ([]LibraryTrack, error) {
	var all []LibraryTrack
	offset := 0

	for page := 0; page < maxLibraryPages; page++ {
		params := url.Values{}
		params.Set("limit", strconv.Itoa(libraryPageSize))
		params.Set("offset", strconv.Itoa(offset))

		path := "/api/v3/library/albums/" + url.PathEscape(albumID) + "/tracks?" + params.Encode()
		data, err := c.do(ctx, http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		var resp albumTracksResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, fmt.Errorf("decode album tracks: %w", err)
		}
		if len(resp.Items) == 0 {
			return all, nil
		}
		all = append(all, resp.Items...)
		offset += len(resp.Items)
		if resp.Total > 0 && offset >= resp.Total {
			return all, nil
		}
	}
	return nil, fmt.Errorf("album %s has too many tracks to list", albumID)
}

// GetTagEditor returns the editable metadata of one library track.
//
// Fields scoped to the album (notably "album" itself) apply to every track of
// that album, so this is the entry point for renaming an album.
func (c *Client) GetTagEditor(ctx context.Context, trackID string) (*TagEditor, error) {
	path := "/api/v3/library/management/tracks/" + url.PathEscape(trackID) + "/tag-editor"
	data, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var editor TagEditor
	if err := json.Unmarshal(data, &editor); err != nil {
		return nil, fmt.Errorf("decode tag editor: %w", err)
	}
	return &editor, nil
}

// CreateTagEditPreview stages metadata changes and returns the job to confirm.
func (c *Client) CreateTagEditPreview(ctx context.Context, request TagEditPreviewRequest) (*PreviewRef, error) {
	data, err := c.do(ctx, http.MethodPost, "/api/v3/library/management/tag-edit-previews", request)
	if err != nil {
		return nil, err
	}
	var ref PreviewRef
	if err := json.Unmarshal(data, &ref); err != nil {
		return nil, fmt.Errorf("decode tag edit preview: %w", err)
	}
	if ref.JobID == "" {
		return nil, errors.New("the server did not return a preview job")
	}
	return &ref, nil
}

// GetManagementPreview returns the state of a staged management job.
func (c *Client) GetManagementPreview(ctx context.Context, jobID string) (*ManagementPreview, error) {
	path := "/api/v3/library/management/previews/" + url.PathEscape(jobID)
	data, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var preview ManagementPreview
	if err := json.Unmarshal(data, &preview); err != nil {
		return nil, fmt.Errorf("decode management preview: %w", err)
	}
	return &preview, nil
}

// ApplyManagementPreview confirms a staged job and queues the work.
func (c *Client) ApplyManagementPreview(ctx context.Context, jobID string, request ApplyPreviewRequest) (*OperationRef, error) {
	path := "/api/v3/library/management/previews/" + url.PathEscape(jobID) + "/apply"
	data, err := c.do(ctx, http.MethodPost, path, request)
	if err != nil {
		return nil, err
	}
	var ref OperationRef
	if err := json.Unmarshal(data, &ref); err != nil {
		return nil, fmt.Errorf("decode applied preview: %w", err)
	}
	return &ref, nil
}

// DiscardManagementPreview abandons a staged job without applying it. The
// endpoint requires the operation row revision the preview was read with.
func (c *Client) DiscardManagementPreview(ctx context.Context, jobID string, expectedRevision json.RawMessage) error {
	if len(expectedRevision) == 0 {
		expectedRevision = json.RawMessage("0")
	}
	body := DiscardPreviewRequest{ExpectedOperationRowRevision: expectedRevision}
	path := "/api/v3/library/management/previews/" + url.PathEscape(jobID) + "/discard"
	_, err := c.do(ctx, http.MethodPost, path, body)
	return err
}

// NewIdempotencyKey returns a random RFC 4122 version 4 UUID for request bodies
// that must not be applied twice.
func NewIdempotencyKey() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate idempotency key: %w", err)
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16]), nil
}

// do sends a JSON request and returns the raw response body.
func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	if body == nil {
		return c.doReader(ctx, method, path, "", nil)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request body: %w", err)
	}
	return c.doReader(ctx, method, path, "application/json", bytes.NewReader(encoded))
}

// doReader sends a request with an arbitrary body and returns the raw response
// body. Keeping the multipart upload on the same path as the JSON calls means
// authentication, timeouts and error decoding are handled in one place.
//
// DroppedNeedle limits how many requests a session may make per second. A 429
// is answered with Retry-After; the request is replayed after that delay (the
// server rejected it, so replaying cannot duplicate a write). Uploads stream a
// pipe and cannot be replayed, so they surface the 429 instead.
func (c *Client) doReader(ctx context.Context, method, path, contentType string, body io.Reader) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", method, path, err)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRateLimitRetries && replayable(body) {
			delay := rateLimitDelay(resp.Header.Get("Retry-After"), attempt)
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseSize))
			resp.Body.Close()
			if err := rewind(body); err != nil {
				return nil, fmt.Errorf("replay %s %s: %w", method, path, err)
			}

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			continue
		}

		data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		resp.Body.Close()
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
}

// Rate-limit backoff: at most a handful of replays, never waiting longer than
// half a minute for one of them.
const (
	maxRateLimitRetries = 5
	rateLimitFallback   = 500 * time.Millisecond
	maxRateLimitDelay   = 30 * time.Second
)

// replayable reports whether a request body can be sent again.
func replayable(body io.Reader) bool {
	if body == nil {
		return true
	}
	_, ok := body.(io.Seeker)
	return ok
}

// rewind puts a replayable body back at its start.
func rewind(body io.Reader) error {
	if body == nil {
		return nil
	}
	seeker, ok := body.(io.Seeker)
	if !ok {
		return errors.New("request body cannot be replayed")
	}
	_, err := seeker.Seek(0, io.SeekStart)
	return err
}

// rateLimitDelay reads Retry-After (whole seconds), falling back to an
// exponential backoff when the server does not send it.
func rateLimitDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds >= 0 {
		delay := time.Duration(seconds) * time.Second
		if delay > maxRateLimitDelay {
			return maxRateLimitDelay
		}
		return delay
	}
	delay := rateLimitFallback << attempt
	if delay > maxRateLimitDelay {
		return maxRateLimitDelay
	}
	return delay
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
