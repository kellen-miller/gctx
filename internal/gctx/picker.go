package gctx

import (
	"context"
	"errors"
	"fmt"
	"strings"

	fzf "github.com/junegunn/fzf/src"
)

const (
	basePickerOptionCount    = 6
	additionalOutputCapacity = 2
)

type contextPicker interface {
	pick(ctx context.Context, labels string, rows []string) (string, error)
}

type fzfPicker struct {
	run     func(*fzf.Options) (int, error)
	options []string
}

func (picker fzfPicker) pick(ctx context.Context, labels string, rows []string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("picker context: %w", err)
	}

	arguments := make([]string, 0, basePickerOptionCount+len(picker.options))
	arguments = append(
		arguments,
		"--delimiter=\t",
		"--with-nth=2",
		"--accept-nth=1",
		"--header="+labels,
		"--prompt=gctx> ",
		"--no-multi",
	)
	arguments = append(arguments, picker.options...)
	options, err := fzf.ParseOptions(false, arguments)
	if err != nil {
		return "", fmt.Errorf("configure embedded fzf: %w", err)
	}

	input := make(chan string, len(rows))
	for _, row := range rows {
		input <- row
	}
	close(input)

	output := make(chan string, len(rows)+additionalOutputCapacity)
	options.Input = input
	options.Output = output

	run := picker.run
	if run == nil {
		run = fzf.Run
	}
	exitCode, err := run(options)
	if err != nil {
		return "", fmt.Errorf("run embedded fzf: %w", err)
	}
	if exitCode == fzf.ExitNoMatch {
		return "", ErrSelectionCanceled
	}
	// fzf handles SIGINT, SIGTERM, and SIGHUP while its terminal is active.
	if exitCode == fzf.ExitInterrupt {
		return "", fmt.Errorf("embedded fzf interrupted: %w", context.Canceled)
	}
	if exitCode != fzf.ExitOk {
		return "", fmt.Errorf("embedded fzf exited with status %d", exitCode)
	}

	if len(output) == 0 {
		return "", errors.New("embedded fzf returned no selection")
	}
	selected := <-output
	if len(output) > 0 {
		return "", errors.New("embedded fzf returned multiple selections")
	}
	if strings.TrimSpace(selected) == "" {
		return "", ErrSelectionCanceled
	}
	return selected, nil
}
