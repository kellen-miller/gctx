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

const developmentVersion = "devel"

// releaseVersion is injected by GoReleaser for archive builds.
var releaseVersion string //nolint:gochecknoglobals // Go linker flags require a package variable.

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	manager := gctx.NewManager(os.Stdin, os.Stdout, os.Stderr)
	code := app.Run(ctx, os.Args, manager, os.Stdout, os.Stderr, buildVersion())
	stop()
	os.Exit(code)
}

func buildVersion() string {
	if releaseVersion != "" {
		return displayVersion(releaseVersion)
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return developmentVersion
	}
	return displayVersion(info.Main.Version)
}

func displayVersion(version string) string {
	if version == "" || version == "(devel)" {
		return developmentVersion
	}
	return version
}
