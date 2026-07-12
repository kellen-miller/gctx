package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"

	"github.com/kellen-miller/gctx/internal/gctx"
)

type contexts interface {
	SelectAndSwitch(ctx context.Context) (gctx.Result, error)
	Switch(ctx context.Context, name string) (gctx.Result, error)
	SwitchPrevious(ctx context.Context) (gctx.Result, error)
	Current(ctx context.Context) (string, error)
}

const (
	exitSuccess     = 0
	exitFailure     = 1
	exitUsage       = 2
	exitInterrupted = 130
)

// Run executes the public command and returns the process exit code.
func Run(ctx context.Context, args []string, useCases contexts, stdout, stderr io.Writer, version string) int {
	command := &cli.Command{
		Name:            "gctx",
		Usage:           "switch Google Cloud CLI contexts",
		UsageText:       "gctx [--current | CONFIGURATION | -]",
		Version:         version,
		Writer:          stdout,
		ErrWriter:       stderr,
		HideHelpCommand: true,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "current",
				Aliases: []string{"c"},
				Usage:   "print the current configuration name",
			},
		},
		ExitErrHandler: func(context.Context, *cli.Command, error) {},
		OnUsageError: func(_ context.Context, _ *cli.Command, err error, _ bool) error {
			return cli.Exit("usage: "+err.Error(), exitUsage)
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			return runAction(ctx, command, useCases, stdout, stderr)
		},
	}

	err := command.Run(ctx, args)
	if err == nil {
		return exitSuccess
	}
	if errors.Is(err, context.Canceled) {
		return exitInterrupted
	}
	if errors.Is(err, gctx.ErrSelectionCanceled) {
		return exitSuccess
	}

	code := exitFailure
	var exitCoder cli.ExitCoder
	if errors.As(err, &exitCoder) {
		code = exitCoder.ExitCode()
	}
	if _, printErr := fmt.Fprintf(stderr, "gctx: %v\n", err); printErr != nil {
		return exitFailure
	}
	return code
}

func runAction(
	ctx context.Context,
	command *cli.Command,
	useCases contexts,
	stdout io.Writer,
	stderr io.Writer,
) error {
	arguments := command.Args()
	if arguments.Len() > 1 || (command.Bool("current") && arguments.Present()) {
		return cli.Exit("usage: gctx [--current | CONFIGURATION | -]", exitUsage)
	}

	if command.Bool("current") {
		name, err := useCases.Current(ctx)
		if err != nil {
			return fmt.Errorf("read current configuration: %w", err)
		}
		if _, err := fmt.Fprintln(stdout, name); err != nil {
			return fmt.Errorf("write current configuration: %w", err)
		}
		return nil
	}

	result, err := dispatch(ctx, arguments.First(), useCases)
	if err != nil || result.Name == "" {
		return err
	}
	if err := writeResult(stdout, &result); err != nil {
		return err
	}
	if result.Warning != "" {
		if _, err := fmt.Fprintf(stderr, "gctx: warning: %s\n", result.Warning); err != nil {
			return fmt.Errorf("write switch warning: %w", err)
		}
	}
	return nil
}

func dispatch(ctx context.Context, name string, useCases contexts) (gctx.Result, error) {
	var (
		result gctx.Result
		err    error
	)
	switch name {
	case "":
		result, err = useCases.SelectAndSwitch(ctx)
	case "-":
		result, err = useCases.SwitchPrevious(ctx)
	default:
		result, err = useCases.Switch(ctx, name)
	}
	if err != nil {
		return gctx.Result{}, fmt.Errorf("switch configuration: %w", err)
	}
	return result, nil
}

func writeResult(writer io.Writer, result *gctx.Result) error {
	_, err := fmt.Fprintf(
		writer,
		"Switched to %s\nAccount: %s\nProject: %s\nQuota project: %s\n",
		result.Name,
		result.Account,
		result.Project,
		result.QuotaProject,
	)
	if err != nil {
		return fmt.Errorf("write switch result: %w", err)
	}
	return nil
}
