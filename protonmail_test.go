package protonmail

import (
	"testing"
	"time"
)

// TestTimestampTime confirms that Timestamp.Time() interprets the int as
// Unix seconds in UTC.
func TestTimestampTime(t *testing.T) {
	cases := []struct {
		name string
		in   Timestamp
		want time.Time
	}{
		{"epoch", 0, time.Unix(0, 0)},
		{"recent", 1700000000, time.Unix(1700000000, 0)},
		{"negative", -100, time.Unix(-100, 0)},
	}
	for _, tc := range cases {
		got := tc.in.Time()
		if !got.Equal(tc.want) {
			t.Errorf("%s: Timestamp(%d).Time() = %v, want %v", tc.name, tc.in, got, tc.want)
		}
	}
}

// TestAPIErrorFormat pins the format of (*APIError).Error(), since it's
// part of the public surface.
func TestAPIErrorFormat(t *testing.T) {
	err := &APIError{Code: 401, Message: "Invalid credentials"}
	want := "[401] Invalid credentials"
	if got := err.Error(); got != want {
		t.Errorf("APIError.Error() = %q, want %q", got, want)
	}
}
