package gctx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Manager owns complete Google Cloud context operations.
type Manager struct {
	runner commandRunner
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// NewManager builds a manager backed by native gcloud and fzf executables.
func NewManager(stdin io.Reader, stdout, stderr io.Writer) *Manager {
	return newManager(execRunner{}, stdin, stdout, stderr)
}

func newManager(runner commandRunner, stdin io.Reader, stdout, stderr io.Writer) *Manager {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return &Manager{runner: runner, stdin: stdin, stdout: stdout, stderr: stderr}
}

// Current returns the active native gcloud configuration name.
func (manager *Manager) Current(ctx context.Context) (string, error) {
	if err := guardEnvironment(); err != nil {
		return "", err
	}
	configurations, err := manager.configurations(ctx)
	if err != nil {
		return "", err
	}
	current, err := currentConfiguration(configurations)
	if err != nil {
		return "", err
	}
	return current.Name, nil
}

// Switch selects one native configuration by name.
func (manager *Manager) Switch(ctx context.Context, name string) (Result, error) {
	if err := guardEnvironment(); err != nil {
		return Result{}, err
	}
	configurations, err := manager.configurations(ctx)
	if err != nil {
		return Result{}, err
	}
	return manager.switchConfiguration(ctx, configurations, name)
}

// SwitchPrevious selects the previously active native configuration.
func (manager *Manager) SwitchPrevious(ctx context.Context) (Result, error) {
	if err := guardEnvironment(); err != nil {
		return Result{}, err
	}
	configurations, err := manager.configurations(ctx)
	if err != nil {
		return Result{}, err
	}
	directory, err := manager.globalConfigDirectory(ctx)
	if err != nil {
		return Result{}, err
	}
	previous, err := readPreviousState(directory)
	if err != nil {
		return Result{}, err
	}
	return manager.switchInDirectory(ctx, configurations, previous, directory)
}

// SelectAndSwitch fuzzy-selects one native configuration.
func (manager *Manager) SelectAndSwitch(ctx context.Context) (Result, error) {
	if err := guardEnvironment(); err != nil {
		return Result{}, err
	}
	configurations, err := manager.configurations(ctx)
	if err != nil {
		return Result{}, err
	}
	var rows strings.Builder
	for _, candidate := range configurations {
		_, _ = fmt.Fprintf(
			&rows,
			"%s\t%s\t%s\t%s\n",
			candidate.Name,
			valueOr(candidate.Properties.Core.Account, "<account unset>"),
			valueOr(candidate.Properties.Core.Project, "<project unset>"),
			valueOr(candidate.Properties.Billing.QuotaProject, "<quota unset>"),
		)
	}
	var selection bytes.Buffer
	err = manager.runner.run(
		ctx,
		"fzf",
		[]string{"--delimiter=\t", "--with-nth=1,2,3,4", "--prompt=gctx> "},
		strings.NewReader(rows.String()),
		&selection,
		manager.stderr,
	)
	if err != nil {
		var exited interface{ ExitCode() int }
		if errors.As(err, &exited) && (exited.ExitCode() == 1 || exited.ExitCode() == 130) && selection.Len() == 0 {
			return Result{}, ErrSelectionCanceled
		}
		return Result{}, fmt.Errorf("run fzf (install fzf or select a configuration directly): %w", err)
	}
	selected := strings.TrimSpace(selection.String())
	if selected == "" {
		return Result{}, ErrSelectionCanceled
	}
	name, _, _ := strings.Cut(selected, "\t")
	if name == "" {
		return Result{}, errors.New("fzf returned a malformed configuration selection")
	}
	return manager.switchConfiguration(ctx, configurations, name)
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func guardEnvironment() error {
	for _, name := range []string{"GOOGLE_APPLICATION_CREDENTIALS", "CLOUDSDK_ACTIVE_CONFIG_NAME"} {
		if os.Getenv(name) != "" {
			return fmt.Errorf("%s is set and would override the context selected by gctx; unset it and retry", name)
		}
	}
	return nil
}

func (manager *Manager) configurations(ctx context.Context) ([]configuration, error) {
	output, err := manager.capture(ctx, "gcloud", []string{"config", "configurations", "list", "--format=" + configurationFormat})
	if err != nil {
		return nil, fmt.Errorf("list gcloud configurations: %w", err)
	}
	return decodeConfigurations(output)
}

func (manager *Manager) globalConfigDirectory(ctx context.Context) (string, error) {
	output, err := manager.capture(ctx, "gcloud", []string{"info", "--format=value(config.paths.global_config_dir)"})
	if err != nil {
		return "", fmt.Errorf("locate the Cloud SDK configuration directory: %w", err)
	}
	directory := strings.TrimSpace(string(output))
	if directory == "" {
		return "", errors.New("gcloud returned an empty Cloud SDK configuration directory")
	}
	return directory, nil
}

func (manager *Manager) switchConfiguration(ctx context.Context, configurations []configuration, name string) (Result, error) {
	target, err := findConfiguration(configurations, name)
	if err != nil {
		return Result{}, err
	}
	if err := validateConfiguration(target); err != nil {
		return Result{}, err
	}
	directory, err := manager.globalConfigDirectory(ctx)
	if err != nil {
		return Result{}, err
	}
	return manager.switchInDirectory(ctx, configurations, name, directory)
}

func (manager *Manager) switchInDirectory(ctx context.Context, configurations []configuration, name, directory string) (Result, error) {
	target, err := findConfiguration(configurations, name)
	if err != nil {
		return Result{}, err
	}
	if err := validateConfiguration(target); err != nil {
		return Result{}, err
	}
	current, currentErr := currentConfiguration(configurations)
	if currentErr != nil {
		current = configuration{}
	}
	var staged *stagedFile
	if current.Name != "" && current.Name != target.Name {
		staged, err = stagePreviousState(directory, current.Name)
		if err != nil {
			return Result{}, err
		}
		defer staged.remove()
	}
	backup, err := backupADC(directory)
	if err != nil {
		return Result{}, err
	}

	rollback := func(primary error) error {
		if restoreErr := backup.restore(); restoreErr != nil {
			return errors.Join(primary, fmt.Errorf("ADC rollback failed; backup retained for manual recovery: %w", restoreErr))
		}
		return primary
	}

	configurationFlag := "--configuration=" + target.Name
	loginArgs := []string{"auth", "login", target.Properties.Core.Account, "--brief", "--no-activate", "--update-adc", configurationFlag}
	if err := manager.runner.run(ctx, "gcloud", loginArgs, manager.stdin, manager.stdout, manager.stderr); err != nil {
		return Result{}, rollback(fmt.Errorf("authenticate %s and update ADC: %w", target.Properties.Core.Account, err))
	}
	if _, err := manager.capture(ctx, "gcloud", []string{"auth", "application-default", "set-quota-project", target.Properties.Billing.QuotaProject, configurationFlag}); err != nil {
		return Result{}, rollback(fmt.Errorf("set ADC quota project %s: %w", target.Properties.Billing.QuotaProject, err))
	}
	if _, err := manager.capture(ctx, "gcloud", []string{"config", "configurations", "activate", target.Name, "--quiet"}); err != nil {
		primary := rollback(fmt.Errorf("activate gcloud configuration %s: %w", target.Name, err))
		if current.Name != "" {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			defer cancel()
			if _, restoreErr := manager.capture(cleanupCtx, "gcloud", []string{"config", "configurations", "activate", current.Name, "--quiet"}); restoreErr != nil {
				primary = errors.Join(primary, fmt.Errorf("restore previous gcloud configuration %s: %w", current.Name, restoreErr))
			}
		}
		return Result{}, primary
	}

	result := resultFor(target)
	if err := staged.commit(); err != nil {
		result.Warning = err.Error() + "; switch succeeded but previous-context history was not updated"
	}
	if err := backup.remove(); err != nil {
		warning := fmt.Sprintf("remove ADC backup %s: %v", backup.temporary, err)
		if result.Warning != "" {
			result.Warning += "; " + warning
		} else {
			result.Warning = warning
		}
	}
	return result, nil
}

func (manager *Manager) capture(ctx context.Context, name string, args []string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	err := manager.runner.run(ctx, name, args, nil, &stdout, &stderr)
	if err != nil {
		diagnostic := strings.TrimSpace(stderr.String())
		if diagnostic != "" {
			return nil, fmt.Errorf("%w: %s", err, diagnostic)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}
