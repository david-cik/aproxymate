package lib

import (
	"fmt"
	"os"

	"aproxymate/lib/logger"
)

// OutputContext combines structured logging with user-friendly console output
type OutputContext struct {
	opCtx *logger.OperationContext
}

// NewOutputContext creates a new output context
func NewOutputContext(opCtx *logger.OperationContext) *OutputContext {
	return &OutputContext{opCtx: opCtx}
}

// Error logs an error both structurally and to the user console
func (oc *OutputContext) Error(msg string, err error, userMsg string, args ...any) {
	// Log structured error
	if oc.opCtx != nil {
		oc.opCtx.Error(msg, err)
	} else {
		logger.Error(msg, "error", err)
	}

	// Print user-friendly message
	fmt.Printf(userMsg, args...)
}

// ErrorAndExit logs an error and exits with code 1
func (oc *OutputContext) ErrorAndExit(msg string, err error, userMsg string, args ...any) {
	oc.Error(msg, err, userMsg, args...)
	os.Exit(1)
}

// Warn logs a warning both structurally and to the user console
func (oc *OutputContext) Warn(msg string, userMsg string, args ...any) {
	// Log structured warning
	if oc.opCtx != nil {
		oc.opCtx.Warn(msg)
	} else {
		logger.Warn(msg)
	}

	// Print user-friendly message
	fmt.Printf(userMsg, args...)
}

// Info logs info both structurally and to the user console
func (oc *OutputContext) Info(msg string, userMsg string, args ...any) {
	// Log structured info
	if oc.opCtx != nil {
		oc.opCtx.Info(msg)
	} else {
		logger.Info(msg)
	}

	// Print user-friendly message
	fmt.Printf(userMsg, args...)
}

// Debug logs debug info both structurally and to the user console (only in debug mode)
func (oc *OutputContext) Debug(msg string, userMsg string, args ...any) {
	// Log structured debug
	if oc.opCtx != nil {
		oc.opCtx.Debug(msg)
	} else {
		logger.Debug(msg)
	}

	// Only print to console in debug mode
	// For now, we'll skip console output for debug to avoid noise
}

// Success logs success both structurally and to the user console
func (oc *OutputContext) Success(msg string, userMsg string, args ...any) {
	// Log structured info for success
	if oc.opCtx != nil {
		oc.opCtx.Info(msg)
	} else {
		logger.Info(msg)
	}

	// Print user-friendly success message
	fmt.Printf(userMsg, args...)
}

// Pure console output without logging
func (oc *OutputContext) Print(msg string, args ...any) {
	fmt.Printf(msg, args...)
}

func (oc *OutputContext) Println(msg string) {
	fmt.Println(msg)
}

// NewSimpleOutputContext creates an output context without operation context
func NewSimpleOutputContext() *OutputContext {
	return &OutputContext{opCtx: nil}
}

// UserError logs a user-friendly error without redundant structured logging
func (oc *OutputContext) UserError(userMsg string, args ...any) {
	fmt.Printf(userMsg, args...)
}

// UserErrorAndExit logs a user-friendly error and exits
func (oc *OutputContext) UserErrorAndExit(userMsg string, args ...any) {
	fmt.Printf(userMsg, args...)
	os.Exit(1)
}

// UserWarn logs a user-friendly warning without redundant structured logging
func (oc *OutputContext) UserWarn(userMsg string, args ...any) {
	fmt.Printf(userMsg, args...)
}

// UserInfo logs a user-friendly info message without redundant structured logging
func (oc *OutputContext) UserInfo(userMsg string, args ...any) {
	fmt.Printf(userMsg, args...)
}
