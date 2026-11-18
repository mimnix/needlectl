// Package mbref turns MusicBrainz references into release-group MBIDs.
//
// DroppedNeedle's held-entry match only accepts a release-group MBID, while what
// people copy from musicbrainz.org is usually a *release* URL. Parse reads
// either form without touching the network; Resolver only asks MusicBrainz when
// the reference is a release (or a bare id whose entity is unknown).
package mbref

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the public MusicBrainz web service.
	DefaultBaseURL = "https://musicbrainz.org"
	// DefaultUserAgent identifies needlectl. MusicBrainz requires a meaningful
	// User-Agent that carries a contact URL.
	DefaultUserAgent = "needlectl (https://github.com/mimmo/needlectl)"

	maxResponseBytes = 1 << 20
	maxRetries       = 2
	maxRetryDelay    = 5 * time.Second
)

// ErrNotFound reports a reference MusicBrainz does not know.
var ErrNotFound = errors.New("not found in MusicBrainz")

// Kind is the MusicBrainz entity a reference points at.
type Kind int

const (
	// KindUnknown is a bare MBID whose entity MusicBrainz has to reveal.
	KindUnknown Kind = iota
	// KindRelease is a release reference.
	KindRelease
	// KindReleaseGroup is a release-group reference.
	KindReleaseGroup
)

// Ref is a parsed MusicBrainz reference.
type Ref struct {
	Kind Kind
	MBID string
}

var mbidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Parse accepts a MusicBrainz release URL, a release-group URL or a bare MBID.
// A bare MBID keeps KindUnknown because only MusicBrainz can say which entity it
// names.
func Parse(raw string) (Ref, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Ref{}, errors.New("MusicBrainz reference is empty")
	}
	if mbidPattern.MatchString(trimmed) {
		return Ref{Kind: KindUnknown, MBID: strings.ToLower(trimmed)}, nil
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || !parsed.IsAbs() {
		return Ref{}, fmt.Errorf("%q is not a MusicBrainz release or release-group URL, nor a MusicBrainz ID", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Ref{}, fmt.Errorf("%q is not a MusicBrainz release or release-group URL, nor a MusicBrainz ID", raw)
	}
	if !isMusicBrainzHost(parsed.Hostname()) {
		return Ref{}, fmt.Errorf("%q is not a musicbrainz.org URL", raw)
	}

	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) != 2 || !mbidPattern.MatchString(segments[1]) {
		return Ref{}, fmt.Errorf("%q is not a MusicBrainz release or release-group URL", raw)
	}

	var kind Kind
	switch segments[0] {
	case "release":
		kind = KindRelease
	case "release-group":
		kind = KindReleaseGroup
	default:
		return Ref{}, fmt.Errorf("%q is not a MusicBrainz release or release-group URL", raw)
	}
	return Ref{Kind: kind, MBID: strings.ToLower(segments[1])}, nil
}

// isMusicBrainzHost accepts musicbrainz.org and its subdomains.
func isMusicBrainzHost(host string) bool {
	host = strings.ToLower(host)
	return host == "musicbrainz.org" || strings.HasSuffix(host, ".musicbrainz.org")
}

// Resolver turns references into release-group MBIDs and tracklists over the
// MusicBrainz web service.
type Resolver struct {
	// BaseURL is the service root, without a trailing slash.
	BaseURL string
	// HTTP is the client used for lookups. A nil client uses http.DefaultClient.
	HTTP *http.Client
	// UserAgent identifies the application, as MusicBrainz requires.
	UserAgent string
	// Pace is the minimum delay between two requests. MusicBrainz asks clients
	// to stay around one request per second; zero disables the delay.
	Pace time.Duration

	lastRequest time.Time
}

// NewResolver returns a resolver for the public MusicBrainz service.
func NewResolver() *Resolver {
	return &Resolver{
		BaseURL:   DefaultBaseURL,
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		UserAgent: DefaultUserAgent,
		Pace:      1100 * time.Millisecond,
	}
}

// Resolve parses raw and returns the release-group MBID it names.
func (r *Resolver) Resolve(ctx context.Context, raw string) (string, error) {
	ref, err := Parse(raw)
	if err != nil {
		return "", err
	}
	return r.ResolveReleaseGroup(ctx, ref)
}

// ResolveReleaseGroup returns the release-group MBID of ref. Release-group
// references are returned unchanged; everything else is looked up.
func (r *Resolver) ResolveReleaseGroup(ctx context.Context, ref Ref) (string, error) {
	switch ref.Kind {
	case KindReleaseGroup:
		return ref.MBID, nil
	case KindRelease:
		return r.releaseGroupOfRelease(ctx, ref.MBID)
	default:
		return r.probeBareMBID(ctx, ref.MBID)
	}
}

// releaseGroupOfRelease looks up a release and returns its release group.
func (r *Resolver) releaseGroupOfRelease(ctx context.Context, releaseMBID string) (string, error) {
	query := url.Values{"inc": {"release-groups"}, "fmt": {"json"}}
	data, err := r.get(ctx, "/ws/2/release/"+url.PathEscape(releaseMBID)+"?"+query.Encode())
	if err != nil {
		return "", err
	}

	var payload struct {
		ReleaseGroup struct {
			ID string `json:"id"`
		} `json:"release-group"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", fmt.Errorf("decode MusicBrainz release %s: %w", releaseMBID, err)
	}
	if !mbidPattern.MatchString(payload.ReleaseGroup.ID) {
		return "", fmt.Errorf("MusicBrainz release %s carries no release group", releaseMBID)
	}
	return strings.ToLower(payload.ReleaseGroup.ID), nil
}

// probeBareMBID decides whether a bare id names a release or a release group.
func (r *Resolver) probeBareMBID(ctx context.Context, mbid string) (string, error) {
	group, err := r.releaseGroupOfRelease(ctx, mbid)
	if err == nil {
		return group, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return "", err
	}

	if _, err := r.get(ctx, "/ws/2/release-group/"+url.PathEscape(mbid)+"?fmt=json"); err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", fmt.Errorf("MusicBrainz knows %s neither as a release nor as a release group", mbid)
		}
		return "", err
	}
	return mbid, nil
}

// get performs one MusicBrainz request, retrying the service-unavailable answer
// MusicBrainz uses for rate limiting.
func (r *Resolver) get(ctx context.Context, path string) ([]byte, error) {
	base := strings.TrimRight(strings.TrimSpace(r.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	client := r.HTTP
	if client == nil {
		client = http.DefaultClient
	}

	for attempt := 0; ; attempt++ {
		r.pause(ctx)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
		if err != nil {
			return nil, fmt.Errorf("build MusicBrainz request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", r.userAgent())

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("call MusicBrainz: %w", err)
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read MusicBrainz response: %w", readErr)
		}

		switch {
		case resp.StatusCode == http.StatusOK:
			return data, nil
		case resp.StatusCode == http.StatusNotFound:
			return nil, fmt.Errorf("%s: %w", path, ErrNotFound)
		case resp.StatusCode == http.StatusServiceUnavailable && attempt < maxRetries:
			waitForRetry(ctx, resp.Header.Get("Retry-After"))
		default:
			return nil, fmt.Errorf("MusicBrainz answered %d for %s", resp.StatusCode, path)
		}
	}
}

// pause waits out the configured request interval.
func (r *Resolver) pause(ctx context.Context) {
	if r.Pace <= 0 {
		return
	}
	wait := time.Duration(0)
	if !r.lastRequest.IsZero() {
		wait = r.Pace - time.Since(r.lastRequest)
	}
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
	r.lastRequest = time.Now()
}

// userAgent returns the configured agent, falling back to the default.
func (r *Resolver) userAgent() string {
	if agent := strings.TrimSpace(r.UserAgent); agent != "" {
		return agent
	}
	return DefaultUserAgent
}

// waitForRetry sleeps for the Retry-After delay, capped so a bad header cannot
// stall the command.
func waitForRetry(ctx context.Context, header string) {
	delay := time.Second
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds >= 0 {
		delay = time.Duration(seconds) * time.Second
	}
	if delay > maxRetryDelay {
		delay = maxRetryDelay
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
