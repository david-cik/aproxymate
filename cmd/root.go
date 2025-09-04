/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"aproxymate/lib"
	log "aproxymate/lib/logger"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "aproxymate",
	Short: "Create Kubernetes proxy pods for remote services",
	Long: `Aproxymate is a tool for creating secure proxy connections to remote services 
running in Kubernetes clusters. It creates temporary proxy pods that forward traffic 
from your local machine to services within the cluster.

Key features:
- Web-based GUI for easy proxy management
- Support for multiple Kubernetes contexts
- Automatic proxy pod lifecycle management
- Configuration file support for persistent setups
- Integration with AWS RDS for automatic endpoint discovery`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip for help commands or when help flags are used
		if cmd.Name() == "help" || cmd.Flags().Changed("help") {
			return nil
		}

		// Get the full command path for context
		commandName := cmd.Use
		if cmd.Parent() != nil && cmd.Parent().Use != "aproxymate" {
			commandName = cmd.Parent().Use + " " + cmd.Use
		}

		// Ensure we have a config or prompt to create one for all commands
		return ensureConfigWithPrompt(commandName)
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Show overview of configuration and suggest next steps
		configFile := viper.ConfigFileUsed()
		if configFile == "" {
			// Try to find and read config file using shared utility
			if foundFile, err := lib.FindAndLoadConfigFile(); err == nil {
				configFile = foundFile
				log.Info("Found and loaded configuration file", "path", configFile)
			}
		}

		if configFile != "" {
			// Convert to absolute path for display
			absPath := lib.GetAbsolutePathForDisplay(configFile)
			fmt.Printf("\nConfiguration file: %s\n", absPath)

			// First, validate the raw YAML file
			yamlData, err := os.ReadFile(configFile)
			if err != nil {
				outputCtx := lib.NewSimpleOutputContext()
				outputCtx.UserError("Error reading configuration file: %v\n", err)

				// Prompt user to select config file location
				location, cancelled, promptErr := lib.PromptConfigLocationTUI()
				if promptErr != nil {
					outputCtx.Error("Failed to prompt for config location", promptErr, "Error occurred\n")
					fmt.Printf("\nFor help with available commands, run: %s --help\n", cmd.CommandPath())
					return
				}

				if cancelled {
					fmt.Println("Configuration file location selection cancelled.")
					fmt.Printf("\nFor help with available commands, run: %s --help\n", cmd.CommandPath())
					return
				}

				// Update configFile to the selected location
				configFile = location
				absPath := lib.GetAbsolutePathForDisplay(configFile)
				fmt.Printf("Selected configuration location: %s\n", absPath)

				// Since no file exists at the selected location, inform user and exit
				fmt.Printf("No configuration file found at: %s\n", absPath)
				fmt.Println("\nTo create a configuration file at this location, run:")
				fmt.Printf("  aproxymate config init --output %s\n", configFile)
				fmt.Printf("\nFor help with available commands, run: %s --help\n", cmd.CommandPath())
				return
			}

			// Validate YAML structure
			if err := lib.ValidateConfigYAML(yamlData); err != nil {
				outputCtx := lib.NewSimpleOutputContext()
				outputCtx.UserError("\nConfiguration validation error: %v\n", err)
				fmt.Println("\nPlease fix this error before continuing.")
				fmt.Printf("For help, run: %s config --help\n", cmd.CommandPath())
				return
			}

			// Try to load and parse the config
			var config lib.AppConfig
			if err := viper.Unmarshal(&config); err != nil {
				outputCtx := lib.NewSimpleOutputContext()
				outputCtx.UserError("Error parsing configuration file: %v\n", err)
				fmt.Printf("\nFor help with available commands, run: %s --help\n", cmd.CommandPath())
				return
			}

			log.LogConfigLoad(absPath, len(config.ProxyConfigs))

			if len(config.ProxyConfigs) > 0 {
				fmt.Printf("\nFound %d proxy configuration(s):\n", len(config.ProxyConfigs))
				fmt.Println(strings.Repeat("-", 40))

				// Check for configurations with missing clusters
				missingClusterConfigs := lib.FindConfigsWithMissingClusters(config.ProxyConfigs)

				for i, proxy := range config.ProxyConfigs {
					fmt.Printf("%d. %s\n", i+1, proxy.Name)
					if proxy.KubernetesCluster == "" {
						fmt.Printf("   Cluster: (not specified) ⚠️\n")
					} else {
						fmt.Printf("   Cluster: %s\n", proxy.KubernetesCluster)
					}
					fmt.Printf("   Remote:  %s:%d\n", proxy.RemoteHost, proxy.RemotePort)
					fmt.Printf("   Local:   localhost:%d\n", proxy.LocalPort)
					if i < len(config.ProxyConfigs)-1 {
						fmt.Println()
					}
				}

				if len(missingClusterConfigs) > 0 {
					fmt.Printf("\n⚠️  %d configuration(s) are missing Kubernetes cluster specifications.\n", len(missingClusterConfigs))
					fmt.Println("To fix this, run:")
					fmt.Printf("  %s config fix\n", cmd.CommandPath())
				} else {
					fmt.Println("\nTo manage these proxies:")
					fmt.Printf("  aproxymate gui --config %s\n", configFile)
				}
			} else {
				fmt.Println("\nNo proxy configurations found in config file.")
				fmt.Printf("\nTo add configurations, run: %s config init\n", cmd.CommandPath())
				fmt.Printf("Or start the GUI: %s gui\n", cmd.CommandPath())
			}
		} else {
			log.Debug("No configuration file found")
			fmt.Println("\nNo configuration file found.")

			// Prompt user to select config file location
			location, cancelled, err := lib.PromptConfigLocationTUI()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				fmt.Printf("\nAlternatively, get started by running: %s config init\n", cmd.CommandPath())
				fmt.Printf("Or start the GUI: %s gui\n", cmd.CommandPath())
			} else if cancelled {
				fmt.Printf("\nGet started by running: %s config init\n", cmd.CommandPath())
				fmt.Printf("Or start the GUI: %s gui\n", cmd.CommandPath())
			} else {
				// User selected a location but no file exists there
				fmt.Printf("Selected configuration location: %s\n", location)
				fmt.Printf("\nTo create a configuration file at this location, run:\n")
				fmt.Printf("  %s config init --output %s\n", cmd.CommandPath(), location)
				fmt.Printf("Or start the GUI: %s gui\n", cmd.CommandPath())
			}
		}

		fmt.Printf("\nFor all available commands, run: %s --help\n", cmd.CommandPath())
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/aproxymate.yaml)")
	rootCmd.PersistentFlags().String("log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("log-format", "text", "log format (text, json)")

	// Bind flags to viper
	viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("log-format", rootCmd.PersistentFlags().Lookup("log-format"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	// Initialize logger based on flags first
	logLevel := viper.GetString("log-level")
	logFormat := viper.GetString("log-format")

	var level log.LogLevel
	switch strings.ToLower(logLevel) {
	case "debug":
		level = log.LevelDebug
	case "info":
		level = log.LevelInfo
	case "warn", "warning":
		level = log.LevelWarn
	case "error":
		level = log.LevelError
	default:
		level = log.LevelInfo
	}

	var format log.LogFormat
	switch strings.ToLower(logFormat) {
	case "json":
		format = log.FormatJSON
	case "text":
		format = log.FormatText
	default:
		format = log.FormatText
	}

	// Use development settings if debug level is enabled
	if level == log.LevelDebug {
		log.InitLogger(log.LoggerConfig{
			Level:         level,
			Format:        format,
			Output:        os.Stderr,
			AddSource:     true,
			IncludeStack:  true,
			MaxStackDepth: 10,
		})
	} else {
		log.InitLogger(log.LoggerConfig{
			Level:         level,
			Format:        format,
			Output:        os.Stderr,
			AddSource:     false,
			IncludeStack:  false,
			MaxStackDepth: 5,
		})
	}

	// Log system information
	log.LogSystemEvent("application_start", "initialization", map[string]any{
		"log_level":  level,
		"log_format": format,
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
	})

	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in multiple locations
		viper.AddConfigPath(home)
		viper.AddConfigPath(".") // Current directory
		viper.SetConfigType("yaml")

		// Try multiple config file names in order
		configNames := []string{"aproxymate", ".aproxymate"}
		var configFound bool

		for _, name := range configNames {
			viper.SetConfigName(name)
			if err := viper.ReadInConfig(); err == nil {
				configFound = true
				break
			}
		}

		if configFound {
			log.Debug("Configuration file loaded via viper", "file", viper.ConfigFileUsed())
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
			return
		}
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if cfgFile != "" {
		if err := viper.ReadInConfig(); err == nil {
			log.Debug("Configuration file loaded via flag", "file", viper.ConfigFileUsed())
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		} else {
			log.Error("Failed to read configuration file", "file", cfgFile, "error", err)

			// Check if the file doesn't exist and offer to create it
			if os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "\nThe specified configuration file does not exist.\n")
				fmt.Fprintf(os.Stderr, "To create a sample configuration file at this location, run:\n")
				fmt.Fprintf(os.Stderr, "  aproxymate config init --output %s\n", cfgFile)
			} else {
				fmt.Fprintf(os.Stderr, "Error reading config file %s: %v\n", cfgFile, err)
				fmt.Fprintf(os.Stderr, "\nPlease check the file permissions and format.\n")
			}
		}
	} else {
		// Print helpful debug info if config file not found
		searchPaths := lib.GetDefaultConfigPaths()

		log.Debug("No configuration file found", "searched_paths", searchPaths)
		fmt.Fprintln(os.Stderr, "Config file not found. Searched locations:")
		for _, path := range searchPaths {
			fmt.Fprintf(os.Stderr, "  %s\n", path)
		}
		fmt.Fprintln(os.Stderr, "Use --config to specify a config file path")
	}
}

// ensureConfigWithPrompt ensures a config file exists or prompts to create one
// This should be called by commands that need a configuration file
func ensureConfigWithPrompt(commandName string) error {
	// Skip config prompting for certain commands that don't need config
	skipCommands := map[string]bool{
		"config init":       true, // init specifically creates config
		"help":              true,
		"--help":            true,
		"-h":                true,
		"completion":        true,
		"config":            false, // Let config subcommands handle individually
		"config show":       false, // Show should prompt to create
		"config list":       false, // List should prompt to create
		"config fix":        false, // Fix should prompt to create
		"config rds-import": false, // rds-import creates config if needed
	}

	// Check if this command should skip config prompting
	if skip, exists := skipCommands[commandName]; exists && skip {
		return nil
	}

	// Check if we already have a config loaded
	if viper.ConfigFileUsed() != "" {
		return nil
	}

	// Try to find a config file first
	if foundFile, err := lib.FindAndLoadConfigFile(); err == nil {
		log.Debug("Found and loaded configuration file", "path", foundFile)
		return nil
	}

	// No config found, prompt user to create one
	log.Debug("No configuration file found, prompting user", "command", commandName)

	location, cancelled, err := lib.PromptConfigLocationTUI()
	if err != nil {
		return fmt.Errorf("failed to prompt for config location: %w", err)
	}

	if cancelled {
		fmt.Println("ℹ️  Continuing without a configuration file.")
		return nil
	}

	// User selected a location - set it for viper but don't create the file
	viper.SetConfigFile(location)
	fmt.Printf("Selected configuration location: %s\n", location)
	fmt.Printf("Note: Configuration file will be used when available at this location.\n")
	fmt.Printf("To create a configuration file now, run: aproxymate config init --output %s\n", location)

	return nil
}
