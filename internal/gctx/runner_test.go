package gctx

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestExecRunnerReturnsContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(25 * time.Millisecond)
		cancel()
	}()

	err := (execRunner{}).run(ctx, "sh", []string{"-c", "sleep 5"}, nil, io.Discard, io.Discard)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
