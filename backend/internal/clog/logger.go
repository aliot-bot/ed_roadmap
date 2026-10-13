package clog

import (
	"io"
	"log/slog"
)

// Logger is the application's structured logging adapter. Callers provide
// stable event names and safe fields; the wrapper owns the slog handler setup.
type Logger struct {
	logger *slog.Logger
}

func New(handler slog.Handler) *Logger {
	if handler == nil {
		handler = slog.NewJSONHandler(io.Discard, nil)
	}
	return &Logger{logger: slog.New(handler)}
}

func NewJSON(w io.Writer, options *slog.HandlerOptions) *Logger {
	return New(slog.NewJSONHandler(w, options))
}

func (l *Logger) Info(event string, args ...any) {
	if l == nil || l.logger == nil {
		return
	}
	l.logger.Info(event, args...)
}

func (l *Logger) Warn(event string, args ...any) {
	if l == nil || l.logger == nil {
		return
	}
	l.logger.Warn(event, args...)
}

func (l *Logger) Error(event string, args ...any) {
	if l == nil || l.logger == nil {
		return
	}
	l.logger.Error(event, args...)
}
