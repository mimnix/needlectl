package output

import "testing"

func TestCountLabelPluralizesNouns(t *testing.T) {
	cases := []struct {
		count    int
		singular string
		want     string
	}{
		{0, "track", "0 tracks"},
		{1, "track", "1 track"},
		{2, "entry", "2 entries"},
		{1, "entry", "1 entry"},
		{2, "job", "2 jobs"},
		{2, "file", "2 files"},
		{2, "album", "2 albums"},
		{3, "day", "3 days"},
	}

	for _, tc := range cases {
		if got := CountLabel(tc.count, tc.singular); got != tc.want {
			t.Errorf("CountLabel(%d, %q) = %q, want %q", tc.count, tc.singular, got, tc.want)
		}
	}
}
