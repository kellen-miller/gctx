package gctx

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

type commandRunner interface {
	run(
		ctx context.Context,
		name string,
		args []string,
		stdin io.Reader,
		stdout io.Writer,
		stderr io.Writer,
	) error
}

type execRunner struct{}

func (execRunner) run(
	ctx context.Context,
	name string,
	args []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("command context: %w", ctxErr)
	}
	if err != nil {
		return fmt.Errorf("execute %s: %w", name, err)
	}
	return nil
}
