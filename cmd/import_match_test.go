package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mimmo/needlectl/internal/api"
)

func TestImportMatchRequiresAMusicBrainzReference(t *testing.T) {
	root := newRootCommand()
	root.SetArgs([]string{"import", "match", "15"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	err := root.Execute()
	if err == nil {
		t.Fatal("import match without --mb returned nil error, want a failure")
	}
	if !strings.Contains(err.Error(), "--mb") {
		t.Errorf("error = %q, want it to mention --mb", err)
	}
}

func TestImportMatchRejectsAllTogetherWithItemIDs(t *testing.T) {
	root := newRootCommand()
	root.SetArgs([]string{"import", "match", "--all", "--mb", testReleaseGroup, "15"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	if err := root.Execute(); err == nil {
		t.Fatal("import match --all with an item id returned nil error, want a failure")
	}
}

func TestImportMatchRequiresATarget(t *testing.T) {
	root := newRootCommand()
	root.SetArgs([]string{"import", "match", "--mb", testReleaseGroup})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	if err := root.Execute(); err == nil {
		t.Fatal("import match without a target returned nil error, want a failure")
	}
}

func TestImportMatchRejectsNonNumericItemIDs(t *testing.T) {
	root := newRootCommand()
	root.SetArgs([]string{"import", "match", "--mb", testReleaseGroup, "not-an-id"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})

	err := root.Execute()
	if err == nil {
		t.Fatal("import match with a non numeric id returned nil error, want a failure")
	}
	if !strings.Contains(err.Error(), "not-an-id") {
		t.Errorf("error = %q, want it to name the offending argument", err)
	}
}

func TestImportMatchSkipsEntriesThatWereAlreadyImported(t *testing.T) {
	printer, _ := testPrinter()
	client, err := api.New("http://127.0.0.1:1", false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	items := []api.ImportItem{
		{ID: 13, Status: api.ImportStatusImported},
		{ID: 14, Status: api.ImportStatusImported},
	}

	// The client points nowhere on purpose: skipping must happen before any
	// request is attempted.
	result := matchImportItems(context.Background(), printer, client, items, testReleaseGroup, false)

	if len(result.Matched) != 0 {
		t.Errorf("Matched = %d entries, want none", len(result.Matched))
	}
	if result.Skipped != 2 {
		t.Errorf("Skipped = %d, want 2", result.Skipped)
	}
	if len(result.Failures) != 0 {
		t.Errorf("Failures = %v, want none", result.Failures)
	}
}

func TestImportMatchDryRunAttemptsNoRequests(t *testing.T) {
	printer, _ := testPrinter()
	client, err := api.New("http://127.0.0.1:1", false)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	items := []api.ImportItem{{ID: 15, Status: api.ImportStatusNeedsReview}}

	result := matchImportItems(context.Background(), printer, client, items, testReleaseGroup, true)

	if len(result.Matched) != 1 {
		t.Errorf("Matched = %d entries, want 1", len(result.Matched))
	}
	if len(result.Failures) != 0 {
		t.Errorf("Failures = %v, want none", result.Failures)
	}
}

func TestHasUnmatchedItems(t *testing.T) {
	cases := []struct {
		name  string
		items []api.ImportItem
		want  bool
	}{
		{"needs review", []api.ImportItem{{Status: api.ImportStatusNeedsReview}}, true},
		{"imported", []api.ImportItem{{Status: api.ImportStatusImported}}, false},
		{"discarded", []api.ImportItem{{Status: api.ImportStatusDiscarded}}, false},
		{"mixed", []api.ImportItem{{Status: api.ImportStatusImported}, {Status: api.ImportStatusNeedsReview}}, true},
		{"empty", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasUnmatchedItems(tc.items); got != tc.want {
				t.Errorf("hasUnmatchedItems(%v) = %v, want %v", tc.items, got, tc.want)
			}
		})
	}
}

func TestPrintMatchSummaryReportsCounts(t *testing.T) {
	printer, out := testPrinter()

	printMatchSummary(printer, matchResult{
		Matched:  []api.ImportItem{{ID: 15}, {ID: 16}},
		Skipped:  3,
		Failures: []string{"item 14: nope"},
	})

	got := out.String()
	for _, want := range []string{"2 entries", "3 already imported", "failed"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q does not contain %q", got, want)
		}
	}
}

func TestPrintMatchSummaryHandlesNothingToDo(t *testing.T) {
	printer, out := testPrinter()

	printMatchSummary(printer, matchResult{Skipped: 2})

	if !strings.Contains(out.String(), "no entry needed matching") {
		t.Errorf("summary %q should report that nothing needed matching", out.String())
	}
}

const testReleaseGroup = "e49c0013-dc74-4fad-b100-ae4d0fe7a1ea"
