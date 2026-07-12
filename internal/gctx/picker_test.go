package gctx

import (
	"context"
	"errors"
	"slices"
	"testing"

	fzf "github.com/junegunn/fzf/src"
)

func TestEmbeddedFZFSelectsMatchingConfiguration(t *testing.T) {
	t.Setenv("FZF_DEFAULT_OPTS", "--invalid-option-that-gctx-must-ignore")
	t.Setenv("FZF_DEFAULT_OPTS_FILE", "")

	selected, err := (fzfPicker{options: []string{"--filter=example-dev"}}).pick(
		t.Context(),
		"CONFIGURATION  ACCOUNT  PROJECT  QUOTA PROJECT",
		[]string{
			"example-old\texample-old  old@example.com    old-project      old-quota",
			"example-dev\texample-dev  user@example.com  example-project  example-quota",
		},
	)

	if err != nil {
		t.Fatalf("pick() error = %v", err)
	}
	want := "example-dev"
	if selected != want {
		t.Fatalf("pick() = %q, want %q", selected, want)
	}
}

func TestEmbeddedFZFUsesFixedLabelsBelowList(t *testing.T) {
	labels := "CONFIGURATION  ACCOUNT  PROJECT  QUOTA PROJECT"
	picker := fzfPicker{run: func(options *fzf.Options) (int, error) {
		if !slices.Equal(options.Header, []string{labels}) {
			t.Fatalf("header = %q, want %q", options.Header, labels)
		}
		if len(options.Footer) != 0 {
			t.Fatalf("footer = %q, want no footer", options.Footer)
		}
		return fzf.ExitNoMatch, nil
	}}

	_, err := picker.pick(t.Context(), labels, []string{"example-dev"})

	if !errors.Is(err, ErrSelectionCanceled) {
		t.Fatalf("pick() error = %v, want ErrSelectionCanceled", err)
	}
}

func TestEmbeddedFZFMapsInterruptToContextCancellation(t *testing.T) {
	picker := fzfPicker{run: func(*fzf.Options) (int, error) {
		return fzf.ExitInterrupt, nil
	}}

	_, err := picker.pick(t.Context(), "", []string{"example-dev"})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pick() error = %v, want context.Canceled", err)
	}
}
