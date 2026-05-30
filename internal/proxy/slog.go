package proxy

import (
	"log/slog"
	"os"
)

// logLevel controls the global slog verbosity.
// Default is Info; SetDebug(true) switches to Debug so slog.Debug output appears.
var logLevel = new(slog.LevelVar)

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	})))
}
