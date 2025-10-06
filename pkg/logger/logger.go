package logger

import (
	"io"
	"log"
	"os"
	"sync"
)

var (
	defaultLogger   *Logger
	defaultLoggerMu sync.RWMutex
)

func init() {
	defaultLogger = NewLogger(false)
}

// Logger provides leveled logging functionality.
type Logger struct {
	debug   *log.Logger
	info    *log.Logger
	warning *log.Logger
	err     *log.Logger
	verbose bool
	prefix  string
	mu      sync.RWMutex
}

// NewLogger creates a new Logger instance.
func NewLogger(verbose bool) *Logger {
	return &Logger{
		debug:   log.New(os.Stderr, "", 0),
		info:    log.New(os.Stdout, "", 0),
		warning: log.New(os.Stderr, "", 0),
		err:     log.New(os.Stderr, "", 0),
		verbose: verbose,
		prefix:  "",
	}
}

// SetVerbose enables or disables verbose logging.
func (l *Logger) SetVerbose(verbose bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.verbose = verbose
}

// SetPrefix sets the prefix for all log messages.
func (l *Logger) SetPrefix(prefix string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prefix = prefix
}

// Clone creates a copy of the logger that can be independently configured.
func (l *Logger) Clone() *Logger {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return &Logger{
		debug:   log.New(l.debug.Writer(), l.debug.Prefix(), l.debug.Flags()),
		info:    log.New(l.info.Writer(), l.info.Prefix(), l.info.Flags()),
		warning: log.New(l.warning.Writer(), l.warning.Prefix(), l.warning.Flags()),
		err:     log.New(l.err.Writer(), l.err.Prefix(), l.err.Flags()),
		verbose: l.verbose,
		prefix:  l.prefix,
	}
}

// SetOutput sets the output destination for all log levels.
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.debug.SetOutput(w)
	l.info.SetOutput(w)
	l.warning.SetOutput(w)
	l.err.SetOutput(w)
}

// SetInfoOutput sets the output destination for info messages.
func (l *Logger) SetInfoOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.info.SetOutput(w)
}

// SetErrorOutput sets the output destination for error, warning, and debug messages.
func (l *Logger) SetErrorOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.debug.SetOutput(w)
	l.warning.SetOutput(w)
	l.err.SetOutput(w)
}

// Debug logs a debug message (only if verbose is enabled).
func (l *Logger) Debug(format string, args ...interface{}) {
	l.mu.RLock()
	verbose := l.verbose
	prefix := l.prefix
	l.mu.RUnlock()
	if verbose {
		l.debug.Printf("[DEBUG] "+prefix+format, args...)
	}
}

// Info logs an informational message.
func (l *Logger) Info(format string, args ...interface{}) {
	l.mu.RLock()
	prefix := l.prefix
	l.mu.RUnlock()
	l.info.Printf(prefix+format, args...)
}

// Warn logs a warning message.
func (l *Logger) Warn(format string, args ...interface{}) {
	l.mu.RLock()
	prefix := l.prefix
	l.mu.RUnlock()
	l.warning.Printf("Warning: "+prefix+format, args...)
}

// Error logs an error message.
func (l *Logger) Error(format string, args ...interface{}) {
	l.mu.RLock()
	prefix := l.prefix
	l.mu.RUnlock()
	l.err.Printf("Error: "+prefix+format, args...)
}

// Default returns the default logger instance.
func Default() *Logger {
	defaultLoggerMu.RLock()
	defer defaultLoggerMu.RUnlock()
	return defaultLogger
}

// SetDefault sets the default logger instance.
func SetDefault(l *Logger) {
	defaultLoggerMu.Lock()
	defer defaultLoggerMu.Unlock()
	defaultLogger = l
}

// SetVerbose enables or disables verbose logging on the default logger.
func SetVerbose(verbose bool) {
	Default().SetVerbose(verbose)
}

// SetOutput sets the output destination for all log levels on the default logger.
func SetOutput(w io.Writer) {
	Default().SetOutput(w)
}

// SetInfoOutput sets the output destination for info messages on the default logger.
func SetInfoOutput(w io.Writer) {
	Default().SetInfoOutput(w)
}

// SetErrorOutput sets the output destination for error, warning, and debug messages on the default logger.
func SetErrorOutput(w io.Writer) {
	Default().SetErrorOutput(w)
}

// Debug logs a debug message using the default logger.
func Debug(format string, args ...interface{}) {
	Default().Debug(format, args...)
}

// Info logs an informational message using the default logger.
func Info(format string, args ...interface{}) {
	Default().Info(format, args...)
}

// Warn logs a warning message using the default logger.
func Warn(format string, args ...interface{}) {
	Default().Warn(format, args...)
}

// Errorf logs an error message using the default logger.
func Errorf(format string, args ...interface{}) {
	Default().Error(format, args...)
}
