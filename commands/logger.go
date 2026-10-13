package commands

import (
	"log/slog"
	"os"
)

var loggerFile *os.File

var SKIP_LOGGER_SETTINGS = false

func SetupLogger(baseFolder string) {
	if SKIP_LOGGER_SETTINGS {
		return
	}
	// TODO: change to size based logger selector
	logFilePath := baseFolder + "/log.log"

	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		err := os.MkdirAll(baseFolder, 0700)
		if err != nil {
			slog.Error("[ShellTime.xyz] failed to create log directory", slog.Any("err", err))
			return
		}

		_, err = os.Create(logFilePath)
		if err != nil {
			slog.Error("[ShellTime.xyz] failed to create log file", slog.Any("err", err))
			return
		}
	}
	// Logs can echo command lines. Tighten a world-readable file once; skip
	// when it is already private so the per-command track path stays cheap.
	if st, err := os.Stat(logFilePath); err == nil && st.Mode().Perm()&0o077 != 0 {
		_ = os.Chmod(logFilePath, 0600)
	}

	f, err := os.OpenFile(logFilePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		os.Stdout.WriteString(err.Error())
		slog.Error("[ShellTime.xyz] on setup logger error", slog.Any("err", err))
		return
	}
	loggerFile = f

	// Create a new slog handler that writes to the file
	handler := slog.NewTextHandler(f, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	})
	slog.SetDefault(slog.New(handler))

	slog.Debug("Setting up logger", slog.String("version", commitID))
}

func CloseLogger() {
	if SKIP_LOGGER_SETTINGS {
		return
	}
	if loggerFile == nil {
		return
	}
	slog.Debug("going to close...")
	loggerFile.Close()
	loggerFile = nil
}
