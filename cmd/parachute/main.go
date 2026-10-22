package main

import (
	"context"
	"crypto/rand"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	_ "time/tzdata"

	"github.com/KazzyAPI/parachute/internal/app"
	"github.com/KazzyAPI/parachute/internal/clock"
	"github.com/KazzyAPI/parachute/internal/osys"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := app.New(app.Dependencies{
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
		System:   osys.New(),
		Clock:    clock.NewSystem(),
		Random:   rand.Reader,
		Version:  buildVersion(),
		Terminal: isTerminal(os.Stderr),
	}).Run(ctx, os.Args[1:], os.Environ())
	stop()
	os.Exit(code)
}

func buildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
