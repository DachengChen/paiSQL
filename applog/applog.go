// Package applog provides general-purpose application logging.
//
// Logs are written to ~/.paisql/logs/app.log with timestamps.
// Covers: app start/stop, config changes, connections, and general events.
package applog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	once    sync.Once
	logFile *os.File
)

func init() {
	once.Do(func() {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return
		}
		logDir := filepath.Join(homeDir, ".paisql", "logs")
		if err := os.MkdirAll(logDir, 0700); err != nil {
			return
		}
		logPath := filepath.Join(logDir, "app.log")
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return
		}
		logFile = f
	})
}

func write(s string) {
	if logFile != nil {
		logFile.WriteString(s) //nolint:errcheck
	}
}

// Info logs a general info message.
func Info(format string, args ...interface{}) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	write(fmt.Sprintf("[%s] INFO  %s\n", ts, msg))
}

// Error logs an error message.
func Error(format string, args ...interface{}) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	write(fmt.Sprintf("[%s] ERROR %s\n", ts, msg))
}

// Event logs a structured event with a category.
func Event(category string, format string, args ...interface{}) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	write(fmt.Sprintf("[%s] %-12s %s\n", ts, category, msg))
}

// Close flushes and closes the log file.
func Close() {
	if logFile != nil {
		logFile.Close()
	}
}
