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

const (
	gcloudCommand              = "gcloud"
	configArgument             = "config"
	configurationsArgument     = "configurations"
	infoArgument               = "info"
	listArgument               = "list"
	authArgument               = "auth"
	loginArgument              = "login"
	activateArgument           = "activate"
	applicationDefaultArgument = "application-default"
	setQuotaProjectArgument    = "set-quota-project"
	briefFlag                  = "--brief"
	noActivateFlag             = "--no-activate"
	updateADCFlag              = "--update-adc"
	verbosityErrorFlag         = "--verbosity=error"
	quietFlag                  = "--quiet"
	cleanupTimeout             = 15 * time.Second
)

// Manager owns complete Google Cloud context operations.
type Manager struct {
	runner commandRunner
	picker contextPicker
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// NewManager builds a manager backed by native gcloud and embedded fzf.
func NewManager(stdin io.Reader, stdout, stderr io.Writer) *Manager {
	return newManagerWithPicker(execRunner{}, fzfPicker{}, stdin, stdout, stderr)
}

func newManager(runner commandRunner, stdout, stderr io.Writer) *Manager {
	return newManagerWithPicker(runner, fzfPicker{}, nil, stdout, stderr)
}

func newManagerWithPicker(
	runner commandRunner,
	picker contextPicker,
	stdin io.Reader,
	stdout, stderr io.Writer,
) *Manager {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return &Manager{runner: runner, picker: picker, stdin: stdin, stdout: stdout, stderr: stderr}
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
	rows := formatConfigurationRows(configurations)
	selected, err := manager.picker.pick(ctx, rows)
	if err != nil {
		if errors.Is(err, ErrSelectionCanceled) {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return Result{}, fmt.Errorf("selection interrupted: %w", ctxErr)
			}
			return Result{}, err
		}
		return Result{}, fmt.Errorf("select gcloud configuration: %w", err)
	}
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return Result{}, ErrSelectionCanceled
	}
	name, _, _ := strings.Cut(selected, "\t")
	if name == "" {
		return Result{}, errors.New("fzf returned a malformed configuration selection")
	}
	return manager.switchConfiguration(ctx, configurations, name)
}

func formatConfigurationRows(configurations []configuration) []string {
	nameWidth, accountWidth, projectWidth := 0, 0, 0
	for _, candidate := range configurations {
		nameWidth = max(nameWidth, len(candidate.Name))
		accountWidth = max(accountWidth, len(valueOr(candidate.Properties.Core.Account, "<account unset>")))
		projectWidth = max(projectWidth, len(valueOr(candidate.Properties.Core.Project, "<project unset>")))
	}

	rows := make([]string, 0, len(configurations))
	for _, candidate := range configurations {
		rows = append(rows, fmt.Sprintf(
			"%s\t%-*s  %-*s  %-*s  %s",
			candidate.Name,
			nameWidth,
			candidate.Name,
			accountWidth,
			valueOr(candidate.Properties.Core.Account, "<account unset>"),
			projectWidth,
			valueOr(candidate.Properties.Core.Project, "<project unset>"),
			valueOr(candidate.Properties.Billing.QuotaProject, "<quota unset>"),
		))
	}
	return rows
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
	output, err := manager.capture(
		ctx,
		[]string{configArgument, configurationsArgument, listArgument, "--format=" + configurationFormat},
	)
	if err != nil {
		return nil, fmt.Errorf("list gcloud configurations: %w", err)
	}
	return decodeConfigurations(output)
}

func (manager *Manager) globalConfigDirectory(ctx context.Context) (string, error) {
	output, err := manager.capture(ctx, []string{infoArgument, "--format=value(config.paths.global_config_dir)"})
	if err != nil {
		return "", fmt.Errorf("locate the Cloud SDK configuration directory: %w", err)
	}
	directory := strings.TrimSpace(string(output))
	if directory == "" {
		return "", errors.New("gcloud returned an empty Cloud SDK configuration directory")
	}
	return directory, nil
}

func (manager *Manager) switchConfiguration(
	ctx context.Context,
	configurations []configuration,
	name string,
) (Result, error) {
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

func (manager *Manager) switchInDirectory(
	ctx context.Context,
	configurations []configuration,
	name, directory string,
) (Result, error) {
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
			return errors.Join(
				primary,
				fmt.Errorf("ADC rollback failed; backup retained for manual recovery: %w", restoreErr),
			)
		}
		return primary
	}

	if err := manager.synchronizeAndActivate(ctx, target, current, rollback); err != nil {
		return Result{}, err
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

func (manager *Manager) synchronizeAndActivate(
	ctx context.Context,
	target configuration,
	current configuration,
	rollback func(error) error,
) error {
	configurationFlag := "--configuration=" + target.Name
	loginArgs := []string{
		authArgument,
		loginArgument,
		target.Properties.Core.Account,
		briefFlag,
		noActivateFlag,
		updateADCFlag,
		verbosityErrorFlag,
		configurationFlag,
	}
	if err := manager.runner.run(
		ctx,
		gcloudCommand,
		loginArgs,
		manager.stdin,
		manager.stdout,
		manager.stderr,
	); err != nil {
		return rollback(fmt.Errorf("authenticate %s and update ADC: %w", target.Properties.Core.Account, err))
	}
	if _, err := manager.capture(ctx, []string{
		authArgument,
		applicationDefaultArgument,
		setQuotaProjectArgument,
		target.Properties.Billing.QuotaProject,
		configurationFlag,
	}); err != nil {
		return rollback(fmt.Errorf("set ADC quota project %s: %w", target.Properties.Billing.QuotaProject, err))
	}
	if _, err := manager.capture(ctx, activationArguments(target.Name)); err != nil {
		return manager.rollbackActivation(ctx, current.Name, target.Name, err, rollback)
	}
	return nil
}

func (manager *Manager) rollbackActivation(
	ctx context.Context,
	currentName string,
	targetName string,
	activationError error,
	rollback func(error) error,
) error {
	primary := rollback(fmt.Errorf("activate gcloud configuration %s: %w", targetName, activationError))
	if currentName == "" {
		return primary
	}

	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()
	if _, err := manager.capture(cleanupCtx, activationArguments(currentName)); err != nil {
		return errors.Join(primary, fmt.Errorf("restore previous gcloud configuration %s: %w", currentName, err))
	}
	return primary
}

func activationArguments(name string) []string {
	return []string{configArgument, configurationsArgument, activateArgument, name, quietFlag}
}

func (manager *Manager) capture(ctx context.Context, args []string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	err := manager.runner.run(ctx, gcloudCommand, args, nil, &stdout, &stderr)
	if err != nil {
		diagnostic := strings.TrimSpace(stderr.String())
		if diagnostic != "" {
			return nil, fmt.Errorf("%w: %s", err, diagnostic)
		}
		return nil, fmt.Errorf("run gcloud: %w", err)
	}
	return stdout.Bytes(), nil
}
