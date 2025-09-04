package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"
)

var (
	// AppLogger is the main application logger
	AppLogger *slog.Logger
	// UILogger is for user-facing output (still using fmt for CLI output)
	// but this can be used for GUI logging
	UILogger *slog.Logger
	// OperationLogger is for tracking operations with context
	OperationLogger *slog.Logger
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

// ContextKey represents context keys for logging
type ContextKey string

const (
	// OperationIDKey is used to track operations across logs
	OperationIDKey ContextKey = "operation_id"
	// ComponentKey identifies the component generating logs
	ComponentKey ContextKey = "component"
	// UserIDKey tracks which user initiated an operation
	UserIDKey ContextKey = "user_id"
)

// LoggerConfig holds configuration for the logger
type LoggerConfig struct {
	Level         LogLevel
	Format        LogFormat
	Output        io.Writer
	AddSource     bool
	IncludeStack  bool
	MaxStackDepth int
}

// OperationContext holds operation-specific logging context
type OperationContext struct {
	ID        string
	Component string
	UserID    string
	StartTime time.Time
	Logger    *slog.Logger
}

// StartOperation creates a new operation context for tracking
func StartOperation(ctx context.Context, component, operation string) (*OperationContext, context.Context) {
	operationID := generateOperationID()
	userID := getUserID()

	logger := AppLogger.With(
		"operation_id", operationID,
		"component", component,
		"operation", operation,
		"user_id", userID,
		"start_time", time.Now().Format(time.RFC3339),
	)

	opCtx := &OperationContext{
		ID:        operationID,
		Component: component,
		UserID:    userID,
		StartTime: time.Now(),
		Logger:    logger,
	}

	newCtx := context.WithValue(ctx, OperationIDKey, operationID)
	newCtx = context.WithValue(newCtx, ComponentKey, component)

	logger.Debug("Operation started", "operation", operation)

	return opCtx, newCtx
}

// Complete marks an operation as completed and logs duration
func (oc *OperationContext) Complete(result string, err error) {
	duration := time.Since(oc.StartTime)

	attrs := []any{
		"result", result,
		"duration_ms", duration.Milliseconds(),
	}

	if err != nil {
		attrs = append(attrs, "error", err.Error())
		if oc.includeStackTrace() {
			attrs = append(attrs, "stack_trace", getStackTrace(5))
		}
		oc.Logger.Debug("Operation completed with error", attrs...)
	} else {
		oc.Logger.Debug("Operation completed successfully", attrs...)
	}
}

// Log logs a message with the operation context
func (oc *OperationContext) Log(level slog.Level, msg string, args ...any) {
	oc.Logger.Log(context.Background(), level, msg, args...)
}

// Debug logs a debug message with operation context
func (oc *OperationContext) Debug(msg string, args ...any) {
	oc.Logger.Debug(msg, args...)
}

// Info logs an info message with operation context
func (oc *OperationContext) Info(msg string, args ...any) {
	oc.Logger.Debug(msg, args...)
}

// Warn logs a warning message with operation context
func (oc *OperationContext) Warn(msg string, args ...any) {
	oc.Logger.Debug(msg, args...)
}

// Error logs an error message with operation context
func (oc *OperationContext) Error(msg string, err error, args ...any) {
	allArgs := make([]any, 0, len(args)+2)
	allArgs = append(allArgs, args...)
	allArgs = append(allArgs, "error", err.Error())

	// Only include stack trace for debug level
	if oc.Logger.Enabled(context.Background(), slog.LevelDebug) && oc.includeStackTrace() {
		allArgs = append(allArgs, "stack_trace", getStackTrace(5))
	}

	oc.Logger.Error(msg, allArgs...)
}

// generateOperationID creates a unique operation ID
func generateOperationID() string {
	return fmt.Sprintf("op_%d_%d", time.Now().UnixNano(), runtime.NumGoroutine())
}

// getUserID gets the current user ID from environment
func getUserID() string {
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	if user := os.Getenv("USERNAME"); user != "" {
		return user
	}
	return "unknown"
}

// includeStackTrace checks if stack traces should be included
func (oc *OperationContext) includeStackTrace() bool {
	// Only include stack trace for debug level
	return oc.Logger.Enabled(context.Background(), slog.LevelDebug)
}

// getStackTrace captures the current stack trace
func getStackTrace(skip int) string {
	var lines []string
	for i := skip; i < skip+10; i++ { // Limit to 10 frames
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}

		fn := runtime.FuncForPC(pc)
		if fn == nil {
			break
		}

		// Shorten file path to just the package and file
		if idx := strings.LastIndex(file, "/"); idx >= 0 {
			if idx2 := strings.LastIndex(file[:idx], "/"); idx2 >= 0 {
				file = file[idx2+1:]
			}
		}

		lines = append(lines, fmt.Sprintf("%s:%d %s", file, line, fn.Name()))
	}

	return strings.Join(lines, " -> ")
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
		Level:     level,
		AddSource: config.AddSource,
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

	// Create operation logger with additional context
	OperationLogger = slog.New(handler).With("logger_type", "operation")

	// Set as default logger
	slog.SetDefault(AppLogger)

	AppLogger.Debug("Logger initialized",
		"level", config.Level,
		"format", config.Format,
		"add_source", config.AddSource,
	)
}

// InitDefaultLogger initializes the logger with sensible defaults
func InitDefaultLogger() {
	InitLogger(LoggerConfig{
		Level:         LevelInfo,
		Format:        FormatText,
		Output:        os.Stderr,
		AddSource:     false,
		IncludeStack:  false,
		MaxStackDepth: 5,
	})
}

// InitDevelopmentLogger initializes logger with development-friendly settings
func InitDevelopmentLogger() {
	InitLogger(LoggerConfig{
		Level:         LevelDebug,
		Format:        FormatText,
		Output:        os.Stderr,
		AddSource:     true,
		IncludeStack:  true,
		MaxStackDepth: 10,
	})
}

// InitProductionLogger initializes logger with production settings
func InitProductionLogger() {
	InitLogger(LoggerConfig{
		Level:         LevelInfo,
		Format:        FormatJSON,
		Output:        os.Stderr,
		AddSource:     false,
		IncludeStack:  false,
		MaxStackDepth: 3,
	})
}

// LogConfigLoad logs configuration loading events with enhanced context
func LogConfigLoad(configFile string, numConfigs int) {
	if configFile != "" {
		absPath, _ := getAbsolutePath(configFile)
		AppLogger.Debug("Configuration loaded successfully",
			"file", configFile,
			"absolute_path", absPath,
			"proxy_configs", numConfigs,
			"component", "config",
		)
	} else {
		AppLogger.Debug("No configuration file found",
			"component", "config",
		)
	}
}

// LogConfigValidation logs configuration validation results with detailed context
func LogConfigValidation(configFile string, err error) {
	absPath, _ := getAbsolutePath(configFile)

	if err != nil {
		AppLogger.Debug("Configuration validation failed",
			"file", configFile,
			"absolute_path", absPath,
			"error", err.Error(),
			"component", "config",
			"validation_result", "failed",
		)
	} else {
		AppLogger.Debug("Configuration validation successful",
			"file", configFile,
			"absolute_path", absPath,
			"component", "config",
			"validation_result", "passed",
		)
	}
}

// LogGUIStart logs GUI startup with server details
func LogGUIStart(port int) {
	AppLogger.Debug("Starting GUI server",
		"port", port,
		"component", "gui",
		"server_type", "http",
		"url", fmt.Sprintf("http://localhost:%d", port),
	)
}

// LogGUIStop logs GUI server shutdown
func LogGUIStop(port int, err error) {
	if err != nil {
		AppLogger.Debug("GUI server stopped with error",
			"port", port,
			"component", "gui",
			"error", err.Error(),
		)
	} else {
		AppLogger.Debug("GUI server stopped gracefully",
			"port", port,
			"component", "gui",
		)
	}
}

// LogKubernetesOperation logs Kubernetes operations with enhanced context
func LogKubernetesOperation(operation string, context string, err error) {
	baseAttrs := []any{
		"operation", operation,
		"kube_context", context,
		"component", "kubernetes",
	}

	if err != nil {
		attrs := append(baseAttrs,
			"error", err.Error(),
			"result", "failed",
		)
		AppLogger.Debug("Kubernetes operation failed", attrs...)
	} else {
		attrs := append(baseAttrs, "result", "success")
		AppLogger.Debug("Kubernetes operation successful", attrs...)
	}
}

// LogKubernetesPodOperation logs pod-specific operations
func LogKubernetesPodOperation(operation, podName, namespace, context string, err error) {
	baseAttrs := []any{
		"operation", operation,
		"pod_name", podName,
		"namespace", namespace,
		"kube_context", context,
		"component", "kubernetes",
		"resource_type", "pod",
	}

	if err != nil {
		attrs := append(baseAttrs,
			"error", err.Error(),
			"result", "failed",
		)
		AppLogger.Debug("Kubernetes pod operation failed", attrs...)
	} else {
		attrs := append(baseAttrs, "result", "success")
		AppLogger.Debug("Kubernetes pod operation successful", attrs...)
	}
}

// LogProxyOperation logs proxy connection operations with comprehensive details
func LogProxyOperation(operation string, cluster string, host string, localPort int, remotePort int, err error) {
	baseAttrs := []any{
		"operation", operation,
		"cluster", cluster,
		"host", host,
		"local_port", localPort,
		"remote_port", remotePort,
		"component", "proxy",
		"proxy_type", "port_forward",
	}

	if err != nil {
		attrs := append(baseAttrs,
			"error", err.Error(),
			"result", "failed",
		)
		AppLogger.Debug("Proxy operation failed", attrs...)
	} else {
		attrs := append(baseAttrs, "result", "success")
		AppLogger.Debug("Proxy operation successful", attrs...)
	}
}

// LogPodCleanup logs pod cleanup operations with namespace details
func LogPodCleanup(operation string, podName string, namespace string, err error) {
	baseAttrs := []any{
		"operation", operation,
		"pod", podName,
		"namespace", namespace,
		"component", "cleanup",
		"resource_type", "pod",
	}

	if err != nil {
		attrs := append(baseAttrs,
			"error", err.Error(),
			"result", "failed",
		)
		AppLogger.Debug("Pod cleanup operation failed", attrs...)
	} else {
		attrs := append(baseAttrs, "result", "success")
		AppLogger.Debug("Pod cleanup operation successful", attrs...)
	}
}

// LogAWSOperation logs AWS-related operations
func LogAWSOperation(operation, region, profile string, err error) {
	baseAttrs := []any{
		"operation", operation,
		"aws_region", region,
		"aws_profile", profile,
		"component", "aws",
	}

	if err != nil {
		attrs := append(baseAttrs,
			"error", err.Error(),
			"result", "failed",
		)
		AppLogger.Debug("AWS operation failed", attrs...)
	} else {
		attrs := append(baseAttrs, "result", "success")
		AppLogger.Debug("AWS operation successful", attrs...)
	}
}

// LogAWSCredentials logs AWS credential validation with security considerations
func LogAWSCredentials(profile, region, accessKeyID string, err error) {
	// Mask the access key for security
	maskedKey := maskAccessKey(accessKeyID)

	baseAttrs := []any{
		"aws_profile", profile,
		"aws_region", region,
		"access_key_id", maskedKey,
		"component", "aws",
		"operation", "credential_validation",
	}

	if err != nil {
		attrs := append(baseAttrs,
			"error", err.Error(),
			"result", "failed",
		)
		AppLogger.Debug("AWS credential validation failed", attrs...)
	} else {
		attrs := append(baseAttrs, "result", "success")
		AppLogger.Debug("AWS credential validation successful", attrs...)
	}
}

// LogFileOperation logs file operations (read, write, delete)
func LogFileOperation(operation, filePath string, size int64, err error) {
	absPath, _ := getAbsolutePath(filePath)

	baseAttrs := []any{
		"operation", operation,
		"file", filePath,
		"absolute_path", absPath,
		"component", "file",
	}

	if size > 0 {
		baseAttrs = append(baseAttrs, "size_bytes", size)
	}

	if err != nil {
		attrs := append(baseAttrs,
			"error", err.Error(),
			"result", "failed",
		)
		AppLogger.Debug("File operation failed", attrs...)
	} else {
		attrs := append(baseAttrs, "result", "success")
		AppLogger.Debug("File operation successful", attrs...)
	}
}

// LogUserAction logs user-initiated actions
func LogUserAction(action, resource string, details map[string]any) {
	attrs := []any{
		"action", action,
		"resource", resource,
		"component", "user_interface",
		"user_id", getUserID(),
		"timestamp", time.Now().Format(time.RFC3339),
	}

	// Add additional details
	for key, value := range details {
		attrs = append(attrs, key, value)
	}

	AppLogger.Debug("User action", attrs...)
}

// LogSystemEvent logs system-level events
func LogSystemEvent(event, category string, details map[string]any) {
	attrs := []any{
		"event", event,
		"category", category,
		"component", "system",
		"timestamp", time.Now().Format(time.RFC3339),
	}

	for key, value := range details {
		attrs = append(attrs, key, value)
	}

	AppLogger.Debug("System event", attrs...)
}

// Helper functions
func getAbsolutePath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	abs, err := os.Getwd()
	if err != nil {
		return path, err
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Sprintf("%s/%s", abs, path), nil
	}
	return path, nil
}

func maskAccessKey(accessKey string) string {
	if len(accessKey) <= 8 {
		return strings.Repeat("*", len(accessKey))
	}
	return accessKey[:4] + strings.Repeat("*", len(accessKey)-8) + accessKey[len(accessKey)-4:]
}

// Debug logs at debug level with caller information
func Debug(msg string, args ...any) {
	if AppLogger.Enabled(context.Background(), slog.LevelDebug) {
		enhancedArgs := addCallerInfo(args)
		AppLogger.Debug(msg, enhancedArgs...)
	}
}

// Info logs at info level
func Info(msg string, args ...any) {
	AppLogger.Info(msg, args...)
}

// Warn logs at warn level with caller information
func Warn(msg string, args ...any) {
	// Only add caller info for debug level
	if AppLogger.Enabled(context.Background(), slog.LevelDebug) {
		enhancedArgs := addCallerInfo(args)
		AppLogger.Warn(msg, enhancedArgs...)
	} else {
		AppLogger.Warn(msg, args...)
	}
}

// Error logs at error level with enhanced error information
func Error(msg string, args ...any) {
	// Only add caller info and stack trace for debug level
	if AppLogger.Enabled(context.Background(), slog.LevelDebug) {
		enhancedArgs := addCallerInfo(args)
		enhancedArgs = addStackTrace(enhancedArgs)
		AppLogger.Error(msg, enhancedArgs...)
	} else {
		AppLogger.Error(msg, args...)
	}
}

// ErrorWithStack logs an error with full stack trace
func ErrorWithStack(msg string, err error, args ...any) {
	allArgs := make([]any, 0, len(args)+6)
	allArgs = append(allArgs, args...)
	allArgs = append(allArgs, "error", err.Error())
	allArgs = append(allArgs, "stack_trace", getStackTrace(3))

	enhancedArgs := addCallerInfo(allArgs)
	AppLogger.Error(msg, enhancedArgs...)
}

// Fatal logs at error level and exits the program
func Fatal(msg string, args ...any) {
	// Only add caller info and stack trace for debug level
	if AppLogger.Enabled(context.Background(), slog.LevelDebug) {
		enhancedArgs := addCallerInfo(args)
		enhancedArgs = addStackTrace(enhancedArgs)
		AppLogger.Error(msg, enhancedArgs...)
	} else {
		AppLogger.Error(msg, args...)
	}
	os.Exit(1)
}

// UserError logs a user-friendly error message without technical details
// This is for errors that should be shown to end users without developer info
func UserError(msg string, err error) {
	if err != nil {
		AppLogger.Error(msg, "error", err.Error())
	} else {
		AppLogger.Error(msg)
	}
}

// Helper functions for enhanced logging
func addCallerInfo(args []any) []any {
	pc, file, line, ok := runtime.Caller(2)
	if !ok {
		return args
	}

	// Shorten file path
	if idx := strings.LastIndex(file, "/"); idx >= 0 {
		if idx2 := strings.LastIndex(file[:idx], "/"); idx2 >= 0 {
			file = file[idx2+1:]
		}
	}

	fn := runtime.FuncForPC(pc)
	var funcName string
	if fn != nil {
		funcName = fn.Name()
		// Shorten function name
		if idx := strings.LastIndex(funcName, "/"); idx >= 0 {
			funcName = funcName[idx+1:]
		}
	}

	enhanced := make([]any, 0, len(args)+6)
	enhanced = append(enhanced, args...)
	enhanced = append(enhanced,
		"caller_file", file,
		"caller_line", line,
		"caller_func", funcName,
	)

	return enhanced
}

func addStackTrace(args []any) []any {
	stack := getStackTrace(3)
	enhanced := make([]any, 0, len(args)+2)
	enhanced = append(enhanced, args...)
	enhanced = append(enhanced, "stack_trace", stack)
	return enhanced
}

// Performance logging helpers
type PerformanceTimer struct {
	name      string
	startTime time.Time
	logger    *slog.Logger
}

// StartTimer creates a new performance timer
func StartTimer(name string) *PerformanceTimer {
	return &PerformanceTimer{
		name:      name,
		startTime: time.Now(),
		logger:    AppLogger.With("component", "performance"),
	}
}

// Stop logs the elapsed time and returns the duration
func (pt *PerformanceTimer) Stop() time.Duration {
	duration := time.Since(pt.startTime)
	pt.logger.Debug("Performance timing",
		"timer_name", pt.name,
		"duration_ms", duration.Milliseconds(),
		"duration_ns", duration.Nanoseconds(),
	)
	return duration
}

// StopWithThreshold logs only if duration exceeds threshold
func (pt *PerformanceTimer) StopWithThreshold(threshold time.Duration) time.Duration {
	duration := time.Since(pt.startTime)
	if duration > threshold {
		pt.logger.Warn("Performance threshold exceeded",
			"timer_name", pt.name,
			"duration_ms", duration.Milliseconds(),
			"threshold_ms", threshold.Milliseconds(),
		)
	} else {
		pt.logger.Debug("Performance timing",
			"timer_name", pt.name,
			"duration_ms", duration.Milliseconds(),
		)
	}
	return duration
}
