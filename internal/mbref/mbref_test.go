package mbref

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	releaseMBID      = "0c641ca7-3d67-4993-81b4-e2d0c2433010"
	releaseGroupMBID = "e49c0013-dc74-4fad-b100-ae4d0fe7a1ea"
)

func TestParseReadsMusicBrainzReferences(t *testing.T) {
	cases := map[string]Ref{
		"https://musicbrainz.org/release/" + releaseMBID:                                {Kind: KindRelease, MBID: releaseMBID},
		"https://musicbrainz.org/release-group/" + releaseGroupMBID:                     {Kind: KindReleaseGroup, MBID: releaseGroupMBID},
		"https://www.musicbrainz.org/release/" + releaseMBID + "/":                      {Kind: KindRelease, MBID: releaseMBID},
		"https://beta.musicbrainz.org/release-group/" + releaseGroupMBID + "?ref=share": {Kind: KindReleaseGroup, MBID: releaseGroupMBID},
		strings.ToUpper(releaseGroupMBID):                                               {Kind: KindUnknown, MBID: releaseGroupMBID},
		"  " + releaseGroupMBID + "  ":                                                  {Kind: KindUnknown, MBID: releaseGroupMBID},
	}

	for input, want := range cases {
		got, err := Parse(input)
		if err != nil {
			t.Errorf("Parse(%q) returned error: %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("Parse(%q) = %+v, want %+v", input, got, want)
		}
	}
}

func TestParseRejectsUnusableReferences(t *testing.T) {
	inputs := []string{
		"",
		"   ",
		"not a reference",
		"https://example.com/release/" + releaseMBID,
		"https://musicbrainz.org/artist/" + releaseGroupMBID,
		"https://musicbrainz.org/release/not-an-mbid",
		"ftp://musicbrainz.org/release/" + releaseMBID,
	}

	for _, input := range inputs {
		if _, err := Parse(input); err == nil {
			t.Errorf("Parse(%q) returned nil error, want a failure", input)
		}
	}
}

func TestResolveReleaseGroupReturnsGroupsWithoutLookup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("release-group references must not be looked up, got %s", r.URL.Path)
	}))
	defer server.Close()

	resolver := testResolver(server)

	group, err := resolver.ResolveReleaseGroup(context.Background(), Ref{Kind: KindReleaseGroup, MBID: releaseGroupMBID})
	if err != nil {
		t.Fatalf("ResolveReleaseGroup returned error: %v", err)
	}
	if group != releaseGroupMBID {
		t.Errorf("group = %q, want %q", group, releaseGroupMBID)
	}
}

func TestResolveReleaseGroupLooksUpARelease(t *testing.T) {
	var requestedURL string
	var userAgent string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedURL = r.URL.String()
		userAgent = r.Header.Get("User-Agent")
		writeJSON(t, w, `{"id":"`+releaseMBID+`","release-group":{"id":"`+releaseGroupMBID+`"}}`)
	}))
	defer server.Close()

	group, err := testResolver(server).Resolve(context.Background(), "https://musicbrainz.org/release/"+releaseMBID)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if group != releaseGroupMBID {
		t.Errorf("group = %q, want %q", group, releaseGroupMBID)
	}
	if !strings.Contains(requestedURL, "/ws/2/release/"+releaseMBID) {
		t.Errorf("requested URL = %q, want the release lookup", requestedURL)
	}
	if !strings.Contains(requestedURL, "inc=release-groups") || !strings.Contains(requestedURL, "fmt=json") {
		t.Errorf("requested URL = %q, want inc=release-groups and fmt=json", requestedURL)
	}
	if userAgent != DefaultUserAgent {
		t.Errorf("User-Agent = %q, want %q", userAgent, DefaultUserAgent)
	}
}

func TestResolveReleaseGroupProbesBareMBIDs(t *testing.T) {
	t.Run("bare id names a release group", func(t *testing.T) {
		var paths []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.URL.Path)
			if strings.HasPrefix(r.URL.Path, "/ws/2/release/") {
				http.NotFound(w, r)
				return
			}
			writeJSON(t, w, `{"id":"`+releaseGroupMBID+`"}`)
		}))
		defer server.Close()

		group, err := testResolver(server).ResolveReleaseGroup(context.Background(), Ref{Kind: KindUnknown, MBID: releaseGroupMBID})
		if err != nil {
			t.Fatalf("ResolveReleaseGroup returned error: %v", err)
		}
		if group != releaseGroupMBID {
			t.Errorf("group = %q, want %q", group, releaseGroupMBID)
		}
		if len(paths) != 2 {
			t.Errorf("paths = %v, want a release probe followed by a release-group probe", paths)
		}
	})

	t.Run("bare id names nothing", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer server.Close()

		_, err := testResolver(server).ResolveReleaseGroup(context.Background(), Ref{Kind: KindUnknown, MBID: releaseGroupMBID})
		if err == nil {
			t.Fatal("ResolveReleaseGroup returned nil error, want a failure")
		}
		if !strings.Contains(err.Error(), "neither as a release nor as a release group") {
			t.Errorf("error = %q, want it to explain that neither entity exists", err)
		}
	})
}

func TestResolveReleaseGroupReportsUnknownReleases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	_, err := testResolver(server).ResolveReleaseGroup(context.Background(), Ref{Kind: KindRelease, MBID: releaseMBID})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestResolveReleaseGroupRejectsMalformedPayloads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, `{"id":"`+releaseMBID+`"}`)
	}))
	defer server.Close()

	_, err := testResolver(server).ResolveReleaseGroup(context.Background(), Ref{Kind: KindRelease, MBID: releaseMBID})
	if err == nil {
		t.Fatal("ResolveReleaseGroup returned nil error, want a failure")
	}
	if !strings.Contains(err.Error(), "carries no release group") {
		t.Errorf("error = %q, want it to mention the missing release group", err)
	}
}

func TestGetRetriesRateLimiting(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writeJSON(t, w, `{"release-group":{"id":"`+releaseGroupMBID+`"}}`)
	}))
	defer server.Close()

	group, err := testResolver(server).ResolveReleaseGroup(context.Background(), Ref{Kind: KindRelease, MBID: releaseMBID})
	if err != nil {
		t.Fatalf("ResolveReleaseGroup returned error: %v", err)
	}
	if group != releaseGroupMBID {
		t.Errorf("group = %q, want %q", group, releaseGroupMBID)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
}

func TestGetGivesUpAfterRepeatedRateLimiting(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if _, err := testResolver(server).ResolveReleaseGroup(context.Background(), Ref{Kind: KindRelease, MBID: releaseMBID}); err == nil {
		t.Fatal("ResolveReleaseGroup returned nil error, want a failure")
	}
	if attempts != maxRetries+1 {
		t.Errorf("attempts = %d, want %d", attempts, maxRetries+1)
	}
}

func TestGetReportsUnexpectedStatuses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := testResolver(server).ResolveReleaseGroup(context.Background(), Ref{Kind: KindRelease, MBID: releaseMBID})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("error = %v, want it to mention status 500", err)
	}
}

// testResolver points a resolver at a test server.
func testResolver(server *httptest.Server) *Resolver {
	return &Resolver{
		BaseURL:   server.URL,
		HTTP:      &http.Client{Timeout: 5 * time.Second},
		UserAgent: DefaultUserAgent,
	}
}

// writeJSON writes a literal JSON response body.
func writeJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := io.WriteString(w, body); err != nil {
		t.Fatalf("write response: %v", err)
	}
}
