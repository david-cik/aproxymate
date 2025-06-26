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
				
				for _, path := range configPaths {
					if _, err := os.Stat(path); err == nil {
						// Found a config file, set it in viper
						viper.SetConfigFile(path)
						if err := viper.ReadInConfig(); err == nil {
							configFile = path
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
			
			// Try to load and parse the config
			var config lib.AppConfig
			if err := viper.Unmarshal(&config); err != nil {
				fmt.Printf("Error parsing configuration file: %v\n", err)
				fmt.Println("\nAvailable commands:")
				fmt.Println("  aproxymate gui      - Start the web GUI")
				fmt.Println("  aproxymate config   - Manage configuration files")
				fmt.Println("  aproxymate --help   - Show help")
				return
			}
			
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
				fmt.Println("\nTo add configurations:")
				fmt.Println("  1. Use the GUI: aproxymate gui")
				fmt.Println("  2. Edit the config file manually")
				fmt.Println("  3. Generate sample config: aproxymate config init")
			}
		} else {
			fmt.Println("\nNo configuration file found.")
			fmt.Println("\nGet started:")
			fmt.Println("  aproxymate config init   - Create a sample configuration")
			fmt.Println("  aproxymate gui           - Start the web GUI")
			fmt.Println("  aproxymate config show   - Show configuration status")
		}
		
		fmt.Println("\nOther commands:")
		fmt.Println("  aproxymate config show   - Show configuration file status")
		fmt.Println("  aproxymate config list   - List all configurations")
		fmt.Println("  aproxymate --help        - Show all available commands")
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
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name "aproxymate" (without extension).
		viper.AddConfigPath(home)
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName("aproxymate")
		viper.SetConfigName(".aproxymate")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
