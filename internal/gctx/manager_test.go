package gctx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type recordedCall struct {
	name string
	args []string
}

type fakeRunner struct {
	calls   []recordedCall
	handler func(name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error
}

type fakePicker struct {
	labels    string
	rows      []string
	selection string
	err       error
}

func (picker *fakePicker) pick(_ context.Context, labels string, rows []string) (string, error) {
	picker.labels = labels
	picker.rows = slices.Clone(rows)
	return picker.selection, picker.err
}

func (f *fakeRunner) run(
	_ context.Context,
	name string,
	args []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
) error {
	f.calls = append(f.calls, recordedCall{name: name, args: slices.Clone(args)})
	if f.handler == nil {
		return nil
	}
	return f.handler(name, args, stdin, stdout, stderr)
}

func TestCurrentReturnsActiveNativeConfiguration(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")

	runner := &fakeRunner{handler: func(_ string, args []string, _ io.Reader, stdout, _ io.Writer) error {
		if args[0] != "config" {
			t.Fatalf("unexpected args: %q", args)
		}
		_, _ = io.WriteString(
			stdout,
			configurationJSON("user@example.com", "example-project", "example-quota"),
		)
		return nil
	}}
	manager := newManager(runner, io.Discard, io.Discard)

	name, err := manager.Current(t.Context())

	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}
	if name != "example-dev" {
		t.Fatalf("Current() = %q, want example-dev", name)
	}
	wantArgs := []string{
		"config",
		"configurations",
		"list",
		"--format=json(name,is_active,properties.core.account,properties.core.project,properties.billing.quota_project)",
	}
	if len(runner.calls) != 1 || !slices.Equal(runner.calls[0].args, wantArgs) {
		t.Fatalf("calls = %#v, want one gcloud %q", runner.calls, wantArgs)
	}
}

func TestEveryOperationRejectsContextOverridesBeforeNativeCommands(t *testing.T) {
	operations := map[string]func(*Manager) error{
		"current":  func(m *Manager) error { _, err := m.Current(t.Context()); return err },
		"direct":   func(m *Manager) error { _, err := m.Switch(t.Context(), "example-dev"); return err },
		"previous": func(m *Manager) error { _, err := m.SwitchPrevious(t.Context()); return err },
		"fuzzy":    func(m *Manager) error { _, err := m.SelectAndSwitch(t.Context()); return err },
	}
	overrides := []string{"GOOGLE_APPLICATION_CREDENTIALS", "CLOUDSDK_ACTIVE_CONFIG_NAME"}

	for _, override := range overrides {
		for name, operation := range operations {
			t.Run(override+"/"+name, func(t *testing.T) {
				t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
				t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
				t.Setenv(override, "set")
				runner := &fakeRunner{}
				manager := newManager(runner, io.Discard, io.Discard)

				err := operation(manager)

				if err == nil || !strings.Contains(err.Error(), override) {
					t.Fatalf("error = %v, want actionable %s rejection", err, override)
				}
				if len(runner.calls) != 0 {
					t.Fatalf("native calls = %#v, want none", runner.calls)
				}
			})
		}
	}
}

func TestSwitchSynchronizesADCBeforeActivation(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	directory := t.TempDir()
	adcPath := filepath.Join(directory, adcFilename)
	priorADC := []byte(`{"type":"authorized_user","refresh_token":"prior-secret"}`)
	if err := os.WriteFile(adcPath, priorADC, 0o600); err != nil {
		t.Fatal(err)
	}

	runner := &fakeRunner{handler: func(_ string, args []string, _ io.Reader, stdout, _ io.Writer) error {
		switch args[0] {
		case "config":
			_, _ = io.WriteString(stdout, "["+
				configurationObject("example-old", true, "old@example.com", "old-project", "old-quota")+","+
				configurationObject("example-dev", false, "user@example.com", "example-project", "example-quota")+"]")
		case "info":
			_, _ = io.WriteString(stdout, directory+"\n")
		case "auth":
			if slices.Equal(
				args[1:],
				[]string{
					"login",
					"user@example.com",
					"--brief",
					"--no-activate",
					"--update-adc",
					"--verbosity=error",
					"--configuration=example-dev",
				},
			) {
				return os.WriteFile(adcPath, []byte("target-without-quota"), 0o600)
			}
			if slices.Equal(
				args[1:],
				[]string{"application-default", "set-quota-project", "example-quota", "--configuration=example-dev"},
			) {
				return os.WriteFile(adcPath, []byte("target-with-quota"), 0o600)
			}
			t.Fatalf("unexpected auth args: %q", args)
		case "configurations":
			t.Fatalf("unexpected malformed args: %q", args)
		}
		return nil
	}}
	manager := newManager(runner, io.Discard, io.Discard)

	result, err := manager.Switch(t.Context(), "example-dev")

	if err != nil {
		t.Fatalf("Switch() error = %v", err)
	}
	if result.Name != "example-dev" || result.QuotaProject != "example-quota" {
		t.Fatalf("result = %#v", result)
	}
	wantCalls := [][]string{
		{
			"config",
			"configurations",
			"list",
			"--format=json(name,is_active,properties.core.account,properties.core.project,properties.billing.quota_project)",
		},
		{"info", "--format=value(config.paths.global_config_dir)"},
		{
			"auth",
			"login",
			"user@example.com",
			"--brief",
			"--no-activate",
			"--update-adc",
			"--verbosity=error",
			"--configuration=example-dev",
		},
		{"auth", "application-default", "set-quota-project", "example-quota", "--configuration=example-dev"},
		{"config", "configurations", "activate", "example-dev", "--quiet"},
	}
	assertCalls(t, runner.calls, wantCalls)

	state, err := os.ReadFile(filepath.Join(directory, stateFilename))
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if string(state) != "{\"previous\":\"example-old\"}\n" {
		t.Fatalf("state = %q", state)
	}
	info, err := os.Stat(filepath.Join(directory, stateFilename))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode = %o, want 600", info.Mode().Perm())
	}
	backups, err := filepath.Glob(filepath.Join(directory, ".gctx-adc-*.tmp"))
	if err != nil || len(backups) != 0 {
		t.Fatalf("ADC backups = %v, err=%v; want none", backups, err)
	}
}

func TestQuotaFailureRestoresExactPriorADC(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	for _, priorExists := range []bool{true, false} {
		t.Run(map[bool]string{true: "existing", false: "absent"}[priorExists], func(t *testing.T) {
			directory := t.TempDir()
			adcPath := filepath.Join(directory, adcFilename)
			priorADC := []byte(`{"refresh_token":"must-not-appear-in-output"}`)
			if priorExists {
				if err := os.WriteFile(adcPath, priorADC, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var stderr bytes.Buffer
			runner := &fakeRunner{handler: func(_ string, args []string, _ io.Reader, stdout, _ io.Writer) error {
				switch args[0] {
				case "config":
					_, _ = io.WriteString(
						stdout,
						"["+configurationObject(
							"example-old",
							true,
							"old@example.com",
							"old-project",
							"old-quota",
						)+","+configurationObject(
							"example-dev",
							false,
							"user@example.com",
							"example-project",
							"example-quota",
						)+"]",
					)
				case "info":
					_, _ = io.WriteString(stdout, directory)
				case "auth":
					if args[1] == "login" {
						return os.WriteFile(adcPath, []byte("target-credential"), 0o600)
					}
					return errors.New("quota denied")
				}
				return nil
			}}
			manager := newManager(runner, io.Discard, &stderr)

			_, err := manager.Switch(t.Context(), "example-dev")

			if err == nil || !strings.Contains(err.Error(), "quota") {
				t.Fatalf("Switch() error = %v, want quota failure", err)
			}
			contents, readErr := os.ReadFile(adcPath)
			if priorExists {
				if readErr != nil || !bytes.Equal(contents, priorADC) {
					t.Fatalf("restored ADC = %q, err=%v; want exact prior bytes", contents, readErr)
				}
			} else if !errors.Is(readErr, os.ErrNotExist) {
				t.Fatalf("new ADC still exists: %q, err=%v", contents, readErr)
			}
			if strings.Contains(stderr.String(), "must-not-appear") ||
				strings.Contains(err.Error(), "must-not-appear") {
				t.Fatal("credential content leaked into output")
			}
			if _, statErr := os.Stat(filepath.Join(directory, stateFilename)); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("state changed on failure: %v", statErr)
			}
		})
	}
}

func TestSwitchRejectsInvalidConfigurationBeforeCredentialChanges(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	tests := []struct {
		name    string
		account string
		project string
		quota   string
		want    string
	}{
		{
			name:    "missing account",
			project: "example-project",
			quota:   "example-quota",
			want:    "gcloud config set account",
		},
		{
			name:    "service account",
			account: "robot@example.iam.gserviceaccount.com",
			project: "example-project",
			quota:   "example-quota",
			want:    "human user accounts only",
		},
		{
			name:    "unsupported principal",
			account: "principal-without-email",
			project: "example-project",
			quota:   "example-quota",
			want:    "human user accounts only",
		},
		{
			name:    "missing project",
			account: "user@example.com",
			quota:   "example-quota",
			want:    "gcloud config set project",
		},
		{
			name:    "missing quota",
			account: "user@example.com",
			project: "example-project",
			want:    "gcloud config set billing/quota_project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &fakeRunner{handler: func(_ string, _ []string, _ io.Reader, stdout, _ io.Writer) error {
				_, _ = io.WriteString(stdout, configurationJSON(tt.account, tt.project, tt.quota))
				return nil
			}}
			manager := newManager(runner, io.Discard, io.Discard)

			_, err := manager.Switch(t.Context(), "example-dev")

			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if len(runner.calls) != 1 {
				t.Fatalf("calls = %#v, want discovery only", runner.calls)
			}
		})
	}
}

func TestSwitchRejectsUnknownConfiguration(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	runner := &fakeRunner{handler: func(_ string, _ []string, _ io.Reader, stdout, _ io.Writer) error {
		_, _ = io.WriteString(
			stdout,
			configurationJSON("user@example.com", "example-project", "example-quota"),
		)
		return nil
	}}
	manager := newManager(runner, io.Discard, io.Discard)

	_, err := manager.Switch(t.Context(), "missing")

	if err == nil || !strings.Contains(err.Error(), `configuration "missing" was not found`) {
		t.Fatalf("error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %#v, want discovery only", runner.calls)
	}
}

func TestActivationFailureRestoresADCAndPreviousConfiguration(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	directory := t.TempDir()
	adcPath := filepath.Join(directory, adcFilename)
	priorADC := []byte("exact-prior-adc")
	if err := os.WriteFile(adcPath, priorADC, 0o600); err != nil {
		t.Fatal(err)
	}
	active := "example-old"
	runner := &fakeRunner{handler: func(_ string, args []string, _ io.Reader, stdout, _ io.Writer) error {
		switch args[0] {
		case "config":
			if args[2] == "list" {
				_, _ = io.WriteString(stdout, "["+
					configurationObject("example-old", true, "old@example.com", "old-project", "old-quota")+","+
					configurationObject(
						"example-dev",
						false,
						"user@example.com",
						"example-project",
						"example-quota",
					)+"]")
				return nil
			}
			active = args[3]
			if active == "example-dev" {
				return errors.New("activation failed after mutation")
			}
		case "info":
			_, _ = io.WriteString(stdout, directory)
		case "auth":
			if args[1] == "login" {
				return os.WriteFile(adcPath, []byte("target-adc"), 0o600)
			}
		}
		return nil
	}}
	manager := newManager(runner, io.Discard, io.Discard)

	_, err := manager.Switch(t.Context(), "example-dev")

	if err == nil || !strings.Contains(err.Error(), "activation failed") {
		t.Fatalf("error = %v, want activation failure", err)
	}
	if active != "example-old" {
		t.Fatalf("active = %q, want previous configuration restored", active)
	}
	contents, readErr := os.ReadFile(adcPath)
	if readErr != nil || !bytes.Equal(contents, priorADC) {
		t.Fatalf("ADC = %q, err=%v; want exact prior bytes", contents, readErr)
	}
	last := runner.calls[len(runner.calls)-1]
	want := []string{"config", "configurations", "activate", "example-old", "--quiet"}
	if last.name != "gcloud" || !slices.Equal(last.args, want) {
		t.Fatalf("last call = %s %q, want gcloud %q", last.name, last.args, want)
	}
}

type cancellationRunner struct {
	cancel    context.CancelFunc
	directory string
	active    string
}

func (runner *cancellationRunner) run(
	ctx context.Context,
	_ string,
	args []string,
	_ io.Reader,
	stdout, _ io.Writer,
) error {
	switch args[0] {
	case "config":
		switch args[2] {
		case "list":
			_, _ = io.WriteString(stdout, "["+
				configurationObject("example-old", true, "old@example.com", "old-project", "old-quota")+","+
				configurationObject("example-dev", false, "user@example.com", "example-project", "example-quota")+"]")
		case "activate":
			if err := ctx.Err(); err != nil {
				return err
			}
			runner.active = args[3]
			if runner.active == "example-dev" {
				runner.cancel()
				return context.Canceled
			}
		}
	case "info":
		_, _ = io.WriteString(stdout, runner.directory)
	case "auth":
		if args[1] == "login" {
			return os.WriteFile(filepath.Join(runner.directory, adcFilename), []byte("target"), 0o600)
		}
	}
	return nil
}

func TestInterruptedActivationUsesCleanupContext(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	directory := t.TempDir()
	adcPath := filepath.Join(directory, adcFilename)
	priorADC := []byte("prior")
	if err := os.WriteFile(adcPath, priorADC, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	runner := &cancellationRunner{cancel: cancel, directory: directory, active: "example-old"}
	manager := newManager(runner, io.Discard, io.Discard)

	_, err := manager.Switch(ctx, "example-dev")

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if runner.active != "example-old" {
		t.Fatalf("active = %q, want cleanup to restore example-old", runner.active)
	}
	contents, readErr := os.ReadFile(adcPath)
	if readErr != nil || !bytes.Equal(contents, priorADC) {
		t.Fatalf("ADC = %q, err=%v; want prior bytes", contents, readErr)
	}
}

func TestStateCommitFailureWarnsWithoutUndoingSwitch(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	directory := t.TempDir()
	adcPath := filepath.Join(directory, adcFilename)
	if err := os.WriteFile(adcPath, []byte("prior"), 0o600); err != nil {
		t.Fatal(err)
	}
	activated := false
	runner := &fakeRunner{handler: func(_ string, args []string, _ io.Reader, stdout, _ io.Writer) error {
		switch args[0] {
		case "config":
			if args[2] == "list" {
				_, _ = io.WriteString(stdout, "["+
					configurationObject("example-old", true, "old@example.com", "old-project", "old-quota")+","+
					configurationObject(
						"example-dev",
						false,
						"user@example.com",
						"example-project",
						"example-quota",
					)+"]")
			} else {
				activated = true
				staged, _ := filepath.Glob(filepath.Join(directory, ".gctx-state-*.tmp"))
				for _, path := range staged {
					_ = os.Remove(path)
				}
			}
		case "info":
			_, _ = io.WriteString(stdout, directory)
		case "auth":
			if args[1] == "login" {
				return os.WriteFile(adcPath, []byte("target"), 0o600)
			}
		}
		return nil
	}}
	manager := newManager(runner, io.Discard, io.Discard)

	result, err := manager.Switch(t.Context(), "example-dev")

	if err != nil {
		t.Fatalf("Switch() error = %v", err)
	}
	if !activated || !strings.Contains(result.Warning, "history was not updated") {
		t.Fatalf("result = %#v, activated=%v", result, activated)
	}
	contents, readErr := os.ReadFile(adcPath)
	if readErr != nil || string(contents) != "target" {
		t.Fatalf("ADC = %q, err=%v; want completed target switch", contents, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(directory, stateFilename)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("state exists after failed commit: %v", statErr)
	}
}

func TestSameConfigurationResynchronizesWithoutChangingPreviousState(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	directory := t.TempDir()
	statePath := filepath.Join(directory, stateFilename)
	initialState := []byte("{\"previous\":\"example-old\"}\n")
	if err := os.WriteFile(statePath, initialState, 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{handler: func(_ string, args []string, _ io.Reader, stdout, _ io.Writer) error {
		switch args[0] {
		case "config":
			if args[2] == "list" {
				_, _ = io.WriteString(
					stdout,
					configurationJSON("user@example.com", "example-project", "example-quota"),
				)
			}
		case "info":
			_, _ = io.WriteString(stdout, directory)
		case "auth":
			if args[1] == "login" {
				return os.WriteFile(filepath.Join(directory, adcFilename), []byte("resynced"), 0o600)
			}
		}
		return nil
	}}
	manager := newManager(runner, io.Discard, io.Discard)

	_, err := manager.Switch(t.Context(), "example-dev")

	if err != nil {
		t.Fatalf("Switch() error = %v", err)
	}
	state, readErr := os.ReadFile(statePath)
	if readErr != nil || !bytes.Equal(state, initialState) {
		t.Fatalf("state = %q, err=%v; want unchanged", state, readErr)
	}
}

func TestRollbackFailureReportsPrimaryAndSafeRecoveryPath(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	directory := t.TempDir()
	adcPath := filepath.Join(directory, adcFilename)
	if err := os.WriteFile(adcPath, []byte("prior-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{handler: func(_ string, args []string, _ io.Reader, stdout, _ io.Writer) error {
		switch args[0] {
		case "config":
			_, _ = io.WriteString(stdout, "["+
				configurationObject("example-old", true, "old@example.com", "old-project", "old-quota")+","+
				configurationObject("example-dev", false, "user@example.com", "example-project", "example-quota")+"]")
		case "info":
			_, _ = io.WriteString(stdout, directory)
		case "auth":
			if args[1] == "login" {
				return os.WriteFile(adcPath, []byte("target"), 0o600)
			}
			backups, _ := filepath.Glob(filepath.Join(directory, ".gctx-adc-*.tmp"))
			for _, path := range backups {
				_ = os.Remove(path)
			}
			return errors.New("quota denied")
		}
		return nil
	}}
	manager := newManager(runner, io.Discard, io.Discard)

	_, err := manager.Switch(t.Context(), "example-dev")

	if err == nil || !strings.Contains(err.Error(), "quota denied") ||
		!strings.Contains(err.Error(), "ADC rollback failed") {
		t.Fatalf("error = %v, want primary and rollback failures", err)
	}
	if strings.Contains(err.Error(), "prior-secret") {
		t.Fatal("error leaked credential contents")
	}
}

func TestCurrentRejectsMalformedNativeOutput(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	runner := &fakeRunner{handler: func(_ string, _ []string, _ io.Reader, stdout, _ io.Writer) error {
		_, _ = io.WriteString(stdout, "not-json")
		return nil
	}}
	manager := newManager(runner, io.Discard, io.Discard)

	_, err := manager.Current(t.Context())

	if err == nil || !strings.Contains(err.Error(), "decode gcloud configurations") {
		t.Fatalf("error = %v, want decode failure", err)
	}
}

func TestSelectAndSwitchUsesEmbeddedFZFAndStrictSwitchPath(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	directory := t.TempDir()
	runner := &fakeRunner{handler: func(name string, args []string, _ io.Reader, stdout, _ io.Writer) error {
		if name != "gcloud" {
			t.Fatalf("external command = %q, want only gcloud", name)
		}
		switch args[0] {
		case "config":
			if args[1] == "configurations" && args[2] == "list" {
				_, _ = io.WriteString(stdout, "["+
					configurationObject("incomplete", false, "", "", "")+","+
					configurationObject("example-old", true, "old@example.com", "old-project", "old-quota")+","+
					configurationObject(
						"example-dev",
						false,
						"user@example.com",
						"example-project",
						"example-quota",
					)+"]")
			}
		case "info":
			_, _ = io.WriteString(stdout, directory)
		case "auth":
			if args[1] == "login" {
				return os.WriteFile(filepath.Join(directory, adcFilename), []byte("target"), 0o600)
			}
		}
		return nil
	}}
	picker := &fakePicker{selection: "example-dev"}
	manager := newManagerWithPicker(runner, picker, strings.NewReader(""), io.Discard, io.Discard)

	result, err := manager.SelectAndSwitch(t.Context())

	if err != nil {
		t.Fatalf("SelectAndSwitch() error = %v", err)
	}
	if result.Name != "example-dev" {
		t.Fatalf("result = %#v", result)
	}
	wantLabels := fmt.Sprintf(
		"%-13s  %-16s  %-15s  %s",
		"CONFIGURATION",
		"ACCOUNT",
		"PROJECT",
		"QUOTA PROJECT",
	)
	if picker.labels != wantLabels {
		t.Fatalf("picker labels = %q, want %q", picker.labels, wantLabels)
	}
	for _, row := range picker.rows {
		if strings.Count(row, "\t") != 1 {
			t.Fatalf("picker row = %q, want one hidden-key delimiter", row)
		}
	}
	wantRows := []string{
		"incomplete\t" + fmt.Sprintf(
			"%-13s  %-16s  %-15s  %s",
			"incomplete",
			"<account unset>",
			"<project unset>",
			"<quota unset>",
		),
		"example-old\t" + fmt.Sprintf(
			"%-13s  %-16s  %-15s  %s",
			"example-old",
			"old@example.com",
			"old-project",
			"old-quota",
		),
		"example-dev\t" + fmt.Sprintf(
			"%-13s  %-16s  %-15s  %s",
			"example-dev",
			"user@example.com",
			"example-project",
			"example-quota",
		),
	}
	if !slices.Equal(picker.rows, wantRows) {
		t.Fatalf("picker rows = %q, want aligned rows %q", picker.rows, wantRows)
	}
	listCalls := 0
	for _, call := range runner.calls {
		if call.name == "gcloud" && len(call.args) > 2 && call.args[0] == "config" && call.args[2] == "list" {
			listCalls++
		}
	}
	if listCalls != 1 {
		t.Fatalf("configuration list calls = %d, want 1", listCalls)
	}
}

func TestSelectAndSwitchTreatsFZFCancellationAsNoOp(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	runner := &fakeRunner{handler: func(name string, _ []string, _ io.Reader, stdout, _ io.Writer) error {
		if name != "gcloud" {
			t.Fatalf("external command = %q, want only gcloud", name)
		}
		_, _ = io.WriteString(
			stdout,
			configurationJSON("user@example.com", "example-project", "example-quota"),
		)
		return nil
	}}
	manager := newManagerWithPicker(
		runner,
		&fakePicker{err: ErrSelectionCanceled},
		nil,
		io.Discard,
		io.Discard,
	)

	_, err := manager.SelectAndSwitch(t.Context())

	if !errors.Is(err, ErrSelectionCanceled) {
		t.Fatalf("error = %v, want ErrSelectionCanceled", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %#v, want only configuration discovery", runner.calls)
	}
}

func TestSelectAndSwitchPreservesInterruptedContext(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	runner := &fakeRunner{handler: func(_ string, _ []string, _ io.Reader, stdout, _ io.Writer) error {
		_, _ = io.WriteString(
			stdout,
			configurationJSON("user@example.com", "example-project", "example-quota"),
		)
		return nil
	}}
	manager := newManagerWithPicker(
		runner,
		&fakePicker{err: ErrSelectionCanceled},
		nil,
		io.Discard,
		io.Discard,
	)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := manager.SelectAndSwitch(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestSwitchPreviousTogglesLikeKubectx(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	directory := t.TempDir()
	statePath := filepath.Join(directory, stateFilename)
	if err := os.WriteFile(statePath, []byte("{\"previous\":\"example-old\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	active := "example-dev"
	runner := &fakeRunner{handler: func(_ string, args []string, _ io.Reader, stdout, _ io.Writer) error {
		switch args[0] {
		case "config":
			switch args[2] {
			case "list":
				_, _ = io.WriteString(stdout, "["+
					configurationObject(
						"example-old",
						active == "example-old",
						"old@example.com",
						"old-project",
						"old-quota",
					)+","+
					configurationObject(
						"example-dev",
						active == "example-dev",
						"user@example.com",
						"example-project",
						"example-quota",
					)+"]")
			case "activate":
				active = args[3]
			}
		case "info":
			_, _ = io.WriteString(stdout, directory)
		case "auth":
			if args[1] == "login" {
				return os.WriteFile(filepath.Join(directory, adcFilename), []byte(args[2]), 0o600)
			}
		}
		return nil
	}}
	manager := newManager(runner, io.Discard, io.Discard)

	first, err := manager.SwitchPrevious(t.Context())
	if err != nil {
		t.Fatalf("first SwitchPrevious() error = %v", err)
	}
	if first.Name != "example-old" || active != "example-old" {
		t.Fatalf("first switch = %#v, active=%q", first, active)
	}
	assertPreviousState(t, statePath, "example-dev")

	second, err := manager.SwitchPrevious(t.Context())
	if err != nil {
		t.Fatalf("second SwitchPrevious() error = %v", err)
	}
	if second.Name != "example-dev" || active != "example-dev" {
		t.Fatalf("second switch = %#v, active=%q", second, active)
	}
	assertPreviousState(t, statePath, "example-old")
}

func TestSwitchPreviousRejectsMissingOrCorruptState(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("CLOUDSDK_ACTIVE_CONFIG_NAME", "")
	for name, contents := range map[string]*[]byte{
		"missing": nil,
		"corrupt": new([]byte("not-json")),
		"empty":   new([]byte("{\"previous\":\"\"}")),
	} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			if contents != nil {
				if err := os.WriteFile(filepath.Join(directory, stateFilename), *contents, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			runner := &fakeRunner{handler: func(_ string, args []string, _ io.Reader, stdout, _ io.Writer) error {
				switch args[0] {
				case "config":
					_, _ = io.WriteString(
						stdout,
						configurationJSON("user@example.com", "example-project", "example-quota"),
					)
				case "info":
					_, _ = io.WriteString(stdout, directory)
				}
				return nil
			}}
			manager := newManager(runner, io.Discard, io.Discard)

			_, err := manager.SwitchPrevious(t.Context())

			if err == nil || !strings.Contains(err.Error(), "previous") {
				t.Fatalf("error = %v, want previous-context error", err)
			}
			if len(runner.calls) != 2 {
				t.Fatalf("calls = %#v, want list and info only", runner.calls)
			}
		})
	}
}

func assertPreviousState(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\"previous\":\""+want+"\"}\n" {
		t.Fatalf("state = %q, want previous %q", data, want)
	}
}

func assertCalls(t *testing.T, got []recordedCall, want [][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("calls = %#v, want %q", got, want)
	}
	for index := range want {
		if got[index].name != "gcloud" || !slices.Equal(got[index].args, want[index]) {
			t.Fatalf("call %d = %s %q, want gcloud %q", index, got[index].name, got[index].args, want[index])
		}
	}
}

func configurationJSON(account, project, quota string) string {
	return "[" + configurationObject("example-dev", true, account, project, quota) + "]"
}

func configurationObject(name string, active bool, account, project, quota string) string {
	activeText := "false"
	if active {
		activeText = "true"
	}
	return `{"name":"` + name + `","is_active":` + activeText + `,"properties":{"core":{"account":"` + account + `","project":"` + project + `"},"billing":{"quota_project":"` + quota + `"}}}`
}
