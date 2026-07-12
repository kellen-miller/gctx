package main

import "testing"

func TestDisplayVersionNormalizesLocalBuild(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"":                         "devel",
		"(devel)":                  "devel",
		"v0.0.0-20260712-deadbeef": "v0.0.0-20260712-deadbeef",
		"v1.2.3":                   "v1.2.3",
	}
	for input, want := range tests {
		if got := displayVersion(input); got != want {
			t.Errorf("displayVersion(%q) = %q, want %q", input, got, want)
		}
	}
}
