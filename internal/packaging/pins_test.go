package packaging

import (
	"context"
	"errors"
	"testing"
)

func TestPinsReportVersionAndDigestDelta(t *testing.T) {
	static := func(version, digest string) func() (string, string, error) {
		return func() (string, string, error) { return version, digest, nil }
	}
	latest := func(version, digest string) func(context.Context) (string, string, error) {
		return func(context.Context) (string, string, error) { return version, digest, nil }
	}
	pins, err := Pins(t.Context(), []PinResolver{
		{Package: "current", Pinned: static("1.0", "aa"), Latest: latest("1.0", "aa")},
		{Package: "stale", Pinned: static("1.0", "aa"), Latest: latest("1.1", "bb")},
		{Package: "digest-drift", Pinned: static("1.0", "aa"), Latest: latest("1.0", "bb")},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"current": "current", "stale": "stale", "digest-drift": "digest drift"}
	for _, pin := range pins {
		if pin.Status() != want[pin.Package] {
			t.Fatalf("%s: got status %q, want %q", pin.Package, pin.Status(), want[pin.Package])
		}
	}
}

func TestPinsFailClosed(t *testing.T) {
	failing := func(context.Context) (string, string, error) { return "", "", errors.New("network") }
	if _, err := Pins(t.Context(), []PinResolver{{Package: "x", Pinned: func() (string, string, error) { return "1", "a", nil }, Latest: failing}}); err == nil {
		t.Fatal("a resolution failure must abort the report")
	}
	if _, err := Pins(t.Context(), []PinResolver{{Package: "x", Pinned: func() (string, string, error) { return "", "", errors.New("bad pin") }, Latest: failing}}); err == nil {
		t.Fatal("an invalid packaged pin must abort the report")
	}
}
