package lib

import (
	"context"
	"io"
	"log/slog"
	"os"
)

var (
	// AppLogger is the main application logger
	AppLogger *slog.Logger
	// UILogger is for user-facing output (still using fmt for CLI output)
	// but this can be used for GUI logging
	UILogger *slog.Logger
)

// LogLevel represents the logging level
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// LogFormat represents the logging format
type LogFormat string

const (
	FormatText LogFormat = "text"
	FormatJSON LogFormat = "json"
)

// LoggerConfig holds configuration for the logger
type LoggerConfig struct {
	Level  LogLevel
	Format LogFormat
	Output io.Writer
}

// InitLogger initializes the application logger with the given configuration
func InitLogger(config LoggerConfig) {
	var level slog.Level
	switch config.Level {
	case LevelDebug:
		level = slog.LevelDebug
	case LevelInfo:
		level = slog.LevelInfo
	case LevelWarn:
		level = slog.LevelWarn
	case LevelError:
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	output := config.Output
	if output == nil {
		output = os.Stderr
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: level,
	}

	switch config.Format {
	case FormatJSON:
		handler = slog.NewJSONHandler(output, opts)
	default:
		handler = slog.NewTextHandler(output, opts)
	}

	AppLogger = slog.New(handler)
	
	// Create a separate logger for UI operations that might need different handling
	UILogger = slog.New(handler)
	
	// Set as default logger
	slog.SetDefault(AppLogger)
}

// InitDefaultLogger initializes the logger with sensible defaults
func InitDefaultLogger() {
	InitLogger(LoggerConfig{
		Level:  LevelInfo,
		Format: FormatText,
		Output: os.Stderr,
	})
}

// LogConfigLoad logs configuration loading events
func LogConfigLoad(configFile string, numConfigs int) {
	if configFile != "" {
		AppLogger.Info("Configuration loaded",
			"file", configFile,
			"proxy_configs", numConfigs)
	} else {
		AppLogger.Debug("No configuration file found")
	}
}

// LogConfigValidation logs configuration validation results
func LogConfigValidation(configFile string, err error) {
	if err != nil {
		AppLogger.Error("Configuration validation failed",
			"file", configFile,
			"error", err)
	} else {
		AppLogger.Debug("Configuration validation successful",
			"file", configFile)
	}
}

// LogGUIStart logs GUI startup
func LogGUIStart(port int) {
	AppLogger.Info("Starting GUI server",
		"port", port)
}

// LogKubernetesOperation logs Kubernetes operations
func LogKubernetesOperation(operation string, context string, err error) {
	if err != nil {
		AppLogger.Error("Kubernetes operation failed",
			"operation", operation,
			"context", context,
			"error", err)
	} else {
		AppLogger.Info("Kubernetes operation successful",
			"operation", operation,
			"context", context)
	}
}

// LogProxyOperation logs proxy connection operations
func LogProxyOperation(operation string, cluster string, host string, localPort int, remotePort int, err error) {
	if err != nil {
		AppLogger.Error("Proxy operation failed",
			"operation", operation,
			"cluster", cluster,
			"host", host,
			"local_port", localPort,
			"remote_port", remotePort,
			"error", err)
	} else {
		AppLogger.Info("Proxy operation successful",
			"operation", operation,
			"cluster", cluster,
			"host", host,
			"local_port", localPort,
			"remote_port", remotePort)
	}
}

// LogPodCleanup logs pod cleanup operations
func LogPodCleanup(operation string, podName string, namespace string, err error) {
	if err != nil {
		AppLogger.Warn("Pod cleanup operation failed",
			"operation", operation,
			"pod", podName,
			"namespace", namespace,
			"error", err)
	} else {
		AppLogger.Info("Pod cleanup operation successful",
			"operation", operation,
			"pod", podName,
			"namespace", namespace)
	}
}

// LogProcessMonitor logs process monitoring events
func LogProcessMonitor(processType string, pid int, status string) {
	AppLogger.Debug("Process status update",
		"type", processType,
		"pid", pid,
		"status", status)
}

// WithContext returns a logger with context
func WithContext(ctx context.Context) *slog.Logger {
	return AppLogger.With()
}

// Debug logs at debug level
func Debug(msg string, args ...any) {
	AppLogger.Debug(msg, args...)
}

// Info logs at info level
func Info(msg string, args ...any) {
	AppLogger.Info(msg, args...)
}

// Warn logs at warn level
func Warn(msg string, args ...any) {
	AppLogger.Warn(msg, args...)
}

// Error logs at error level
func Error(msg string, args ...any) {
	AppLogger.Error(msg, args...)
}
