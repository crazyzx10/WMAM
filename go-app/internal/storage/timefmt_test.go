package storage

import "testing"

func TestFormatDisplayTimeConvertsRFC3339UTCToEastEight(t *testing.T) {
	got := formatDisplayTime("2026-05-26T00:54:24Z")
	want := "2026-05-26 08:54:24"
	if got != want {
		t.Fatalf("formatDisplayTime() = %q, want %q", got, want)
	}
}

func TestFormatDisplayTimeTreatsSQLiteTimestampAsUTC(t *testing.T) {
	got := formatDisplayTime("2026-05-26 00:54:28")
	want := "2026-05-26 08:54:28"
	if got != want {
		t.Fatalf("formatDisplayTime() = %q, want %q", got, want)
	}
}
