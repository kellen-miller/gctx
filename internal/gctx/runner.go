package gctx

import (
	"context"
	"io"
	"os/exec"
)

type commandRunner interface {
	run(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error
}

type execRunner struct{}

func (execRunner) run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return err
}
