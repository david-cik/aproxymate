/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"aproxymate/cmd"
	"aproxymate/lib/logger"
)

func main() {
	// Initialize logger with default settings
	logger.InitDefaultLogger()
	
	cmd.Execute()
}
