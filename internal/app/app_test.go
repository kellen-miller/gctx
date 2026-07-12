package app_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kellen-miller/gctx/internal/app"
	"github.com/kellen-miller/gctx/internal/gctx"
)

type fakeContexts struct {
	called  string
	name    string
	result  gctx.Result
	current string
	err     error
}

func (f *fakeContexts) SelectAndSwitch(context.Context) (gctx.Result, error) {
	f.called = "select"
	return f.result, f.err
}

func (f *fakeContexts) Switch(_ context.Context, name string) (gctx.Result, error) {
	f.called = "switch"
	f.name = name
	return f.result, f.err
}

func (f *fakeContexts) SwitchPrevious(context.Context) (gctx.Result, error) {
	f.called = "previous"
	return f.result, f.err
}

func (f *fakeContexts) Current(context.Context) (string, error) {
	f.called = "current"
	return f.current, f.err
}

func TestRunDispatchesAndPrintsSwitchSummary(t *testing.T) {
	t.Parallel()

	useCases := &fakeContexts{result: gctx.Result{
		Name:         "example-dev",
		Account:      "user@example.com",
		Project:      "example-dev",
		QuotaProject: "example-quota",
	}}
	var stdout, stderr bytes.Buffer

	code := app.Run(t.Context(), []string{"gctx", "example-dev"}, useCases, &stdout, &stderr, "test")

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if useCases.called != "switch" || useCases.name != "example-dev" {
		t.Fatalf("dispatch = %q(%q), want switch(example-dev)", useCases.called, useCases.name)
	}
	want := "Switched to example-dev\nAccount: user@example.com\nProject: example-dev\nQuota project: example-quota\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunDispatchesPublicForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		current string
		called  string
		stdout  string
	}{
		{name: "fuzzy", args: []string{"gctx"}, called: "select"},
		{name: "previous", args: []string{"gctx", "-"}, called: "previous"},
		{
			name:    "current long",
			args:    []string{"gctx", "--current"},
			current: "example-dev",
			called:  "current",
			stdout:  "example-dev\n",
		},
		{
			name:    "current short",
			args:    []string{"gctx", "-c"},
			current: "example-dev",
			called:  "current",
			stdout:  "example-dev\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			useCases := &fakeContexts{current: tt.current}
			var stdout, stderr bytes.Buffer

			code := app.Run(t.Context(), tt.args, useCases, &stdout, &stderr, "test")

			if code != 0 {
				t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
			}
			if useCases.called != tt.called {
				t.Fatalf("called = %q, want %q", useCases.called, tt.called)
			}
			if stdout.String() != tt.stdout {
				t.Fatalf("stdout = %q, want %q", stdout.String(), tt.stdout)
			}
		})
	}
}

func TestRunRejectsAmbiguousArguments(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{"gctx", "one", "two"},
		{"gctx", "--current", "one"},
	} {
		useCases := &fakeContexts{}
		var stdout, stderr bytes.Buffer

		code := app.Run(t.Context(), args, useCases, &stdout, &stderr, "test")

		if code != 2 {
			t.Fatalf("Run(%q) exit code = %d, want 2", args, code)
		}
		if useCases.called != "" {
			t.Fatalf("Run(%q) dispatched %q, want no operation", args, useCases.called)
		}
		if !strings.Contains(stderr.String(), "usage") {
			t.Fatalf("Run(%q) stderr = %q, want usage error", args, stderr.String())
		}
	}
}

func TestRunTreatsSelectionCancellationAsSuccess(t *testing.T) {
	t.Parallel()

	useCases := &fakeContexts{err: gctx.ErrSelectionCanceled}
	var stdout, stderr bytes.Buffer

	code := app.Run(t.Context(), []string{"gctx"}, useCases, &stdout, &stderr, "test")

	if code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("cancel result = (%d, %q, %q), want quiet success", code, stdout.String(), stderr.String())
	}
}

func TestRunTreatsInterruptedSelectionAsInterrupt(t *testing.T) {
	t.Parallel()

	useCases := &fakeContexts{err: errors.Join(gctx.ErrSelectionCanceled, context.Canceled)}
	var stdout, stderr bytes.Buffer

	code := app.Run(t.Context(), []string{"gctx"}, useCases, &stdout, &stderr, "test")

	if code != 130 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("interrupt result = (%d, %q, %q), want quiet exit 130", code, stdout.String(), stderr.String())
	}
}

func TestRunReportsOperationError(t *testing.T) {
	t.Parallel()

	useCases := &fakeContexts{err: errors.New("native failure")}
	var stdout, stderr bytes.Buffer

	code := app.Run(t.Context(), []string{"gctx", "example-dev"}, useCases, &stdout, &stderr, "test")

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if stderr.String() != "gctx: switch configuration: native failure\n" {
		t.Fatalf("stderr = %q, want contextual error", stderr.String())
	}
}

func TestRunPrintsNonFatalSwitchWarning(t *testing.T) {
	t.Parallel()

	useCases := &fakeContexts{result: gctx.Result{
		Name:         "example-dev",
		Account:      "user@example.com",
		Project:      "example-project",
		QuotaProject: "example-quota",
		Warning:      "previous-context history was not updated",
	}}
	var stdout, stderr bytes.Buffer

	code := app.Run(t.Context(), []string{"gctx", "example-dev"}, useCases, &stdout, &stderr, "test")

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stderr.String() != "gctx: warning: previous-context history was not updated\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunAllowsConfigurationNamedHelp(t *testing.T) {
	t.Parallel()

	useCases := &fakeContexts{}
	var stdout, stderr bytes.Buffer

	code := app.Run(t.Context(), []string{"gctx", "help"}, useCases, &stdout, &stderr, "test")

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if useCases.called != "switch" || useCases.name != "help" {
		t.Fatalf("dispatch = %q(%q), want switch(help)", useCases.called, useCases.name)
	}
}

func TestRunMapsUnknownFlagToUsageError(t *testing.T) {
	t.Parallel()

	useCases := &fakeContexts{}
	var stdout, stderr bytes.Buffer

	code := app.Run(t.Context(), []string{"gctx", "--bogus"}, useCases, &stdout, &stderr, "test")

	if code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr=%q", code, stderr.String())
	}
	if useCases.called != "" {
		t.Fatalf("dispatched %q, want no operation", useCases.called)
	}
}
