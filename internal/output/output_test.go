package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrinterWritesStepsAndSuccess(t *testing.T) {
	var out, errOut bytes.Buffer
	printer := New(&out, &errOut, Options{})

	printer.Step("loading %s", "library")
	printer.Success("done")
	printer.Info("extra detail")

	got := out.String()
	for _, want := range []string{"loading library", "done", "extra detail"} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, "\033[") {
		t.Errorf("output %q contains ANSI escapes while colour is disabled", got)
	}
}

func TestCountLabelUsesSingularAndPlural(t *testing.T) {
	cases := map[int]string{0: "0 tracks", 1: "1 track", 2: "2 tracks"}
	for count, want := range cases {
		if got := CountLabel(count, "track"); got != want {
			t.Errorf("CountLabel(%d, track) = %q, want %q", count, got, want)
		}
	}
}

func TestErrorsGoToStderr(t *testing.T) {
	var out, errOut bytes.Buffer
	printer := New(&out, &errOut, Options{})

	printer.Error("boom")
	printer.Warn("careful")

	if strings.Contains(out.String(), "boom") || strings.Contains(out.String(), "careful") {
		t.Errorf("stdout %q should not contain error or warning output", out.String())
	}
	if !strings.Contains(errOut.String(), "boom") || !strings.Contains(errOut.String(), "careful") {
		t.Errorf("stderr %q is missing the error or warning", errOut.String())
	}
}

func TestDebugIsOnlyPrintedWhenVerbose(t *testing.T) {
	var quiet, verbose bytes.Buffer

	New(&quiet, &quiet, Options{}).Debug("hidden")
	if strings.Contains(quiet.String(), "hidden") {
		t.Errorf("non-verbose output %q contains debug output", quiet.String())
	}

	New(&verbose, &verbose, Options{Verbose: true}).Debug("visible")
	if !strings.Contains(verbose.String(), "visible") {
		t.Errorf("verbose output %q is missing debug output", verbose.String())
	}
}

func TestColorsEnabledHonoursTheFlag(t *testing.T) {
	if ColorsEnabled(nil, true) {
		t.Error("ColorsEnabled(nil, true) = true, want false")
	}
	if ColorsEnabled(nil, false) {
		t.Error("ColorsEnabled(nil, false) = true, want false for a nil file")
	}
}

func TestColorIsAppliedWhenEnabled(t *testing.T) {
	var out bytes.Buffer
	printer := New(&out, &out, Options{Color: true})

	printer.Success("done")
	if !strings.Contains(out.String(), "\033[") {
		t.Errorf("output %q does not contain ANSI escapes while colour is enabled", out.String())
	}
}

func TestFieldAlignsLabel(t *testing.T) {
	var out bytes.Buffer
	printer := New(&out, &out, Options{})

	printer.Field("playlist", "Anime")
	if !strings.Contains(out.String(), "playlist:") || !strings.Contains(out.String(), "Anime") {
		t.Errorf("output %q is missing the field", out.String())
	}
}
