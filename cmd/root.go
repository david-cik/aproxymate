/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
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
	Long: `Aproxymate creates socat proxy pods in Kubernetes clusters to help
establish connections to remote services.

Aproxymate makes it easy to set up temporary proxies using socat, 
allowing you to connect to remote services through Kubernetes pods.`,
	Run: func(cmd *cobra.Command, args []string) {
		// When called without subcommands, show configuration status and list configs
		fmt.Println("🚀 Aproxymate - Kubernetes Proxy Manager")
		fmt.Println("=======================================")
		
		// Check if a config file exists and load it
		configFile := viper.ConfigFileUsed()
		if configFile == "" {
			// Try to find and read config file manually
			home, err := os.UserHomeDir()
			if err == nil {
				// Check common config file locations
				configPaths := []string{
					filepath.Join(home, "aproxymate.yaml"),
					filepath.Join(home, ".aproxymate.yaml"),
					"./aproxymate.yaml",
					"./.aproxymate.yaml",
				}
				
				log.Debug("Searching for configuration files", "paths", configPaths)
				
				for _, path := range configPaths {
					if _, err := os.Stat(path); err == nil {
						// Found a config file, set it in viper
						viper.SetConfigFile(path)
						if err := viper.ReadInConfig(); err == nil {
							configFile = path
							log.Info("Found and loaded configuration file", "path", path)
							break
						}
					}
				}
			}
		}
		
		if configFile != "" {
			// Convert to absolute path for display
			absPath, err := filepath.Abs(configFile)
			if err != nil {
				absPath = configFile
			}
			fmt.Printf("\nConfiguration file: %s\n", absPath)
			
			// First, validate the raw YAML file
			yamlData, err := os.ReadFile(configFile)
			if err != nil {
				log.Error("Failed to read configuration file", "file", absPath, "error", err)
				fmt.Printf("Error reading configuration file: %v\n", err)
				fmt.Printf("\nFor help with available commands, run: %s --help\n", cmd.CommandPath())
				return
			}
					// Validate YAML structure
		if err := lib.ValidateConfigYAML(yamlData); err != nil {
			log.Error("Configuration validation failed", "file", absPath, "error", err)
			fmt.Printf("\n❌ Configuration validation error: %v\n", err)
			fmt.Println("\nPlease fix this error before continuing.")
			fmt.Printf("For help, run: %s config --help\n", cmd.CommandPath())
			return
		}
		
		// Try to load and parse the config
		var config lib.AppConfig
			if err := viper.Unmarshal(&config); err != nil {
				log.Error("Failed to parse configuration file", "file", absPath, "error", err)
				fmt.Printf("Error parsing configuration file: %v\n", err)
				fmt.Printf("\nFor help with available commands, run: %s --help\n", cmd.CommandPath())
				return
			}
			
			log.LogConfigLoad(absPath, len(config.ProxyConfigs))
			
			if len(config.ProxyConfigs) > 0 {
				fmt.Printf("\nFound %d proxy configuration(s):\n", len(config.ProxyConfigs))
				fmt.Println(strings.Repeat("-", 40))
				
				for i, proxy := range config.ProxyConfigs {
					fmt.Printf("%d. %s\n", i+1, proxy.Name)
					fmt.Printf("   Cluster: %s\n", proxy.KubernetesCluster)
					fmt.Printf("   Remote:  %s:%d\n", proxy.RemoteHost, proxy.RemotePort)
					fmt.Printf("   Local:   localhost:%d\n", proxy.LocalPort)
					if i < len(config.ProxyConfigs)-1 {
						fmt.Println()
					}
				}
				
				fmt.Println("\nTo manage these proxies:")
				fmt.Printf("  aproxymate gui --config %s\n", configFile)
			} else {
				fmt.Println("\nNo proxy configurations found in config file.")
				fmt.Printf("\nTo add configurations, run: %s config init\n", cmd.CommandPath())
				fmt.Printf("Or start the GUI: %s gui\n", cmd.CommandPath())
			}
		} else {
			log.Debug("No configuration file found")
			fmt.Println("\nNo configuration file found.")
			fmt.Printf("\nGet started by running: %s config init\n", cmd.CommandPath())
			fmt.Printf("Or start the GUI: %s gui\n", cmd.CommandPath())
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
	
	log.InitLogger(log.LoggerConfig{
		Level:  level,
		Format: format,
		Output: os.Stderr,
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
		viper.AddConfigPath(".")        // Current directory
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
			fmt.Fprintf(os.Stderr, "Error reading config file %s: %v\n", cfgFile, err)
		}
	} else {
		// Print helpful debug info if config file not found
		searchPaths := []string{
			"./aproxymate.yaml",
			"./.aproxymate.yaml",
		}
		
		home, err := os.UserHomeDir()
		if err == nil {
			searchPaths = append(searchPaths,
				fmt.Sprintf("%s/aproxymate.yaml", home),
				fmt.Sprintf("%s/.aproxymate.yaml", home),
			)
		}
		
		log.Debug("No configuration file found", "searched_paths", searchPaths)
		fmt.Fprintln(os.Stderr, "Config file not found. Searched locations:")
		for _, path := range searchPaths {
			fmt.Fprintf(os.Stderr, "  %s\n", path)
		}
		fmt.Fprintln(os.Stderr, "Use --config to specify a config file path")
	}
}
