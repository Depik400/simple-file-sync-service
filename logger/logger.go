package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogLevel represents the logging level
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

// Logger represents a structured logger
type Logger struct {
	level         LogLevel
	fileWriter    *os.File
	consoleWriter io.Writer
	logFile       string
	buffer        []string
	bufferSize    int
	mutex         sync.RWMutex
	uiMode        bool
}

// NewLogger creates a new logger instance
func NewLogger(configLogFile string, uiMode bool, level LogLevel) (*Logger, error) {
	logger := &Logger{
		level:      level,
		bufferSize: 100, // Keep last 100 log messages for UI
		buffer:     make([]string, 0, 100),
		uiMode:     uiMode,
	}

	// In UI mode, always log to file
	if uiMode || configLogFile != "" {
		logFile := configLogFile
		if logFile == "" {
			logFile = "file-sync.log"
		}

		// Ensure log directory exists
		logDir := filepath.Dir(logFile)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}

		file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}

		logger.fileWriter = file
		logger.logFile = logFile

		// In UI mode, only write to file (not console to avoid conflicts)
		if uiMode {
			logger.consoleWriter = file
		} else {
			logger.consoleWriter = io.MultiWriter(os.Stderr, file)
		}
	} else {
		logger.consoleWriter = os.Stderr
	}

	return logger, nil
}

// Close closes the logger
func (l *Logger) Close() error {
	if l.fileWriter != nil {
		return l.fileWriter.Close()
	}
	return nil
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, "DEBUG", format, args...)
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, "INFO", format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, "WARN", format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, "ERROR", format, args...)
}

// Printf logs a message with printf-style formatting (for compatibility)
func (l *Logger) Printf(format string, args ...interface{}) {
	l.log(INFO, "INFO", format, args...)
}

// log handles the actual logging
func (l *Logger) log(level LogLevel, levelStr, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf(format, args...)

	// Create log entry
	logEntry := fmt.Sprintf("[%s] [%s] %s", timestamp, levelStr, message)

	// Write to console/file
	if l.consoleWriter != nil {
		fmt.Fprintln(l.consoleWriter, logEntry)
	}

	// Store in buffer for UI (keep last N messages)
	l.mutex.Lock()
	l.buffer = append(l.buffer, logEntry)
	if len(l.buffer) > l.bufferSize {
		l.buffer = l.buffer[1:]
	}
	l.mutex.Unlock()
}

// GetRecentLogs returns the last N log messages for UI display
func (l *Logger) GetRecentLogs(count int) []string {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	if len(l.buffer) <= count {
		return append([]string(nil), l.buffer...)
	}

	return append([]string(nil), l.buffer[len(l.buffer)-count:]...)
}

// GetLogFile returns the path to the log file
func (l *Logger) GetLogFile() string {
	return l.logFile
}

// SetUIMode sets the UI mode (affects where logs are written)
func (l *Logger) SetUIMode(uiMode bool) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.uiMode = uiMode
}

// Global logger instance
var defaultLogger *Logger

// InitDefaultLogger initializes the default logger
func InitDefaultLogger(configLogFile string, uiMode bool) error {
	level := INFO
	if uiMode {
		level = DEBUG // More verbose in UI mode
	}

	logger, err := NewLogger(configLogFile, uiMode, level)
	if err != nil {
		return err
	}

	defaultLogger = logger
	return nil
}

// GetDefaultLogger returns the default logger
func GetDefaultLogger() *Logger {
	return defaultLogger
}

// Convenience functions for global logging
func Debug(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Debug(format, args...)
	}
}

func Info(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Info(format, args...)
	}
}

func Warn(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Warn(format, args...)
	}
}

func Error(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Error(format, args...)
	}
}

func Printf(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Printf(format, args...)
	}
}

// SetLevel sets the logging level
func SetLevel(level LogLevel) {
	if defaultLogger != nil {
		defaultLogger.mutex.Lock()
		defaultLogger.level = level
		defaultLogger.mutex.Unlock()
	}
}

// GetRecentLogs returns recent logs from default logger
func GetRecentLogs(count int) []string {
	if defaultLogger != nil {
		return defaultLogger.GetRecentLogs(count)
	}
	return []string{}
}

// Close closes the default logger
func Close() error {
	if defaultLogger != nil {
		return defaultLogger.Close()
	}
	return nil
}
