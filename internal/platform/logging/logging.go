// Package logging configures the process-wide structured logger (slog).
package logging

import (
	"log/slog"
	"os"
)

// Setup installs a JSON slog handler at the given level ("debug", "info",
// "warn", "error") as the default logger for the process.
func Setup(level string) {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(handler))
}
