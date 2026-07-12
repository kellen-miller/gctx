package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/kellen-miller/gctx/internal/gctx"
	"github.com/urfave/cli/v3"
)

type contexts interface {
	SelectAndSwitch(context.Context) (gctx.Result, error)
	Switch(context.Context, string) (gctx.Result, error)
	SwitchPrevious(context.Context) (gctx.Result, error)
	Current(context.Context) (string, error)
}

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
			return cli.Exit("usage: "+err.Error(), 2)
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			arguments := command.Args()
			if arguments.Len() > 1 || (command.Bool("current") && arguments.Present()) {
				return cli.Exit("usage: gctx [--current | CONFIGURATION | -]", 2)
			}

			if command.Bool("current") {
				name, err := useCases.Current(ctx)
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(stdout, name)
				return err
			}

			var (
				result gctx.Result
				err    error
			)
			switch arguments.First() {
			case "":
				result, err = useCases.SelectAndSwitch(ctx)
			case "-":
				result, err = useCases.SwitchPrevious(ctx)
			default:
				result, err = useCases.Switch(ctx, arguments.First())
			}
			if err != nil || result.Name == "" {
				return err
			}
			if err := writeResult(stdout, result); err != nil {
				return err
			}
			if result.Warning != "" {
				_, err = fmt.Fprintf(stderr, "gctx: warning: %s\n", result.Warning)
			}
			return err
		},
	}

	err := command.Run(ctx, args)
	if err == nil || errors.Is(err, gctx.ErrSelectionCanceled) {
		return 0
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}

	code := 1
	var exitCoder cli.ExitCoder
	if errors.As(err, &exitCoder) {
		code = exitCoder.ExitCode()
	}
	_, _ = fmt.Fprintf(stderr, "gctx: %v\n", err)
	return code
}

func writeResult(writer io.Writer, result gctx.Result) error {
	_, err := fmt.Fprintf(
		writer,
		"Switched to %s\nAccount: %s\nProject: %s\nQuota project: %s\n",
		result.Name,
		result.Account,
		result.Project,
		result.QuotaProject,
	)
	return err
}
