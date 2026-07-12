package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/kellen-miller/gctx/internal/app"
	"github.com/kellen-miller/gctx/internal/gctx"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	manager := gctx.NewManager(os.Stdin, os.Stdout, os.Stderr)
	os.Exit(app.Run(ctx, os.Args, manager, os.Stdout, os.Stderr, buildVersion()))
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "devel"
	}
	return displayVersion(info.Main.Version)
}

func displayVersion(version string) string {
	if version == "" || version == "(devel)" {
		return "devel"
	}
	return version
}
