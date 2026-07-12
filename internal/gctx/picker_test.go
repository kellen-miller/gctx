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
		"example-old\told@example.com\told-project\told-quota",
		"example-dev\tuser@example.com\texample-project\texample-quota",
	})

	if err != nil {
		t.Fatalf("pick() error = %v", err)
	}
	want := "example-dev\tuser@example.com\texample-project\texample-quota"
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
