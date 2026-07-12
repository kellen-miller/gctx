package gctx

import (
	"context"
	"errors"
	"testing"

	fzf "github.com/junegunn/fzf/src"
)

func TestEmbeddedFZFSelectsMatchingConfiguration(t *testing.T) {
	t.Setenv("FZF_DEFAULT_OPTS", "--invalid-option-that-gctx-must-ignore")
	t.Setenv("FZF_DEFAULT_OPTS_FILE", "")

	selected, err := (fzfPicker{options: []string{"--filter=example-dev"}}).pick(t.Context(), []string{
		"example-old\texample-old  old@example.com    old-project      old-quota",
		"example-dev\texample-dev  user@example.com  example-project  example-quota",
	})

	if err != nil {
		t.Fatalf("pick() error = %v", err)
	}
	want := "example-dev"
	if selected != want {
		t.Fatalf("pick() = %q, want %q", selected, want)
	}
}

func TestEmbeddedFZFMapsInterruptToContextCancellation(t *testing.T) {
	picker := fzfPicker{run: func(*fzf.Options) (int, error) {
		return fzf.ExitInterrupt, nil
	}}

	_, err := picker.pick(t.Context(), []string{"example-dev"})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pick() error = %v, want context.Canceled", err)
	}
}
