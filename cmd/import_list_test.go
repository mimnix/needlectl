package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mimnix/needlectl/internal/api"
	"github.com/mimnix/needlectl/internal/output"
)

func TestFilterImportItemsByStatus(t *testing.T) {
	items := []api.ImportItem{
		{ID: 13, Status: api.ImportStatusImported},
		{ID: 14, Status: api.ImportStatusImported},
		{ID: 15, Status: api.ImportStatusNeedsReview},
	}

	if got := filterImportItems(items, ""); len(got) != 3 {
		t.Errorf("filterImportItems with no status kept %d entries, want 3", len(got))
	}
	if got := filterImportItems(items, api.ImportStatusNeedsReview); len(got) != 1 || got[0].ID != 15 {
		t.Errorf("filterImportItems(needs_review) = %+v, want only item 15", got)
	}
	if got := filterImportItems(items, "discarded"); len(got) != 0 {
		t.Errorf("filterImportItems(discarded) = %+v, want none", got)
	}
}

func TestCountImportItemsCountsAcrossJobs(t *testing.T) {
	jobs := []api.ImportJob{
		{Items: []api.ImportItem{{Status: api.ImportStatusNeedsReview}, {Status: api.ImportStatusImported}}},
		{Items: []api.ImportItem{{Status: api.ImportStatusNeedsReview}}},
	}

	if got := countImportItems(jobs, api.ImportStatusNeedsReview); got != 2 {
		t.Errorf("countImportItems = %d, want 2", got)
	}
	if got := countImportItems(jobs, ""); got != 3 {
		t.Errorf("countImportItems with no status = %d, want 3", got)
	}
}

func TestImportJobLabelShortensLongIDs(t *testing.T) {
	if got := importJobLabel("461dafd3fd234cd8af9153394e427651"); got != "461dafd3" {
		t.Errorf("importJobLabel = %q, want the first eight characters", got)
	}
	if got := importJobLabel("short"); got != "short" {
		t.Errorf("importJobLabel = %q, want the id unchanged", got)
	}
}

func TestImportItemLabelDescribesEntries(t *testing.T) {
	title := "T.M.R. LIVE REVOLUTION'15 -Strikes Back XI-"
	artist := "T.M.Revolution"

	matched := api.ImportItem{ID: 15, Status: api.ImportStatusImported, AlbumTitle: &title, ArtistName: &artist, FilesTotal: 1, FilesImported: 1}
	if got := importItemLabel(matched); !strings.Contains(got, title) || !strings.Contains(got, "1/1 files") {
		t.Errorf("importItemLabel = %q, want the album and the file count", got)
	}

	unmatched := api.ImportItem{ID: 15, Status: api.ImportStatusNeedsReview, FolderName: "Loose tracks", FilesTotal: 1}
	if got := importItemLabel(unmatched); !strings.Contains(got, "Loose tracks") {
		t.Errorf("importItemLabel = %q, want the folder name as fallback", got)
	}
}

func TestPrintImportJobRendersItems(t *testing.T) {
	printer, out := testPrinter()
	detail := "Couldn't work out which album this is. Match it manually."

	job := api.ImportJob{
		ID:         "461dafd3fd234cd8af9153394e427651",
		Status:     "completed",
		UploadName: "DOUBLE-DEAL (Live) - T.M.Revolution.mp3",
		Items: []api.ImportItem{{
			ID:         15,
			Status:     api.ImportStatusNeedsReview,
			FolderName: "Loose tracks",
			Detail:     &detail,
			FilesTotal: 1,
		}},
	}

	printImportJob(printer, job, job.Items)

	got := out.String()
	for _, want := range []string{"461dafd3", "DOUBLE-DEAL (Live)", "item 15", "needs_review", detail} {
		if !strings.Contains(got, want) {
			t.Errorf("job output %q does not contain %q", got, want)
		}
	}
}

func TestImportListCommandExposesFilters(t *testing.T) {
	jobs := newImportListCommand(&globalOptions{})

	for _, name := range []string{"all", "status"} {
		if jobs.Flags().Lookup(name) == nil {
			t.Errorf("import list does not expose --%s", name)
		}
	}
}

func TestStatusSuffix(t *testing.T) {
	if got := statusSuffix("  "); got != "" {
		t.Errorf("statusSuffix(blank) = %q, want empty", got)
	}
	if got := statusSuffix(" needs_review "); got != " with status needs_review" {
		t.Errorf("statusSuffix = %q, want the trimmed status", got)
	}
}

// testPrinter returns a printer and the buffer it writes to.
func testPrinter() (*output.Printer, *bytes.Buffer) {
	var buffer bytes.Buffer
	return output.New(&buffer, &buffer, output.Options{}), &buffer
}
