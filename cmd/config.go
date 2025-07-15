package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"aproxymate/lib"
	log "aproxymate/lib/logger"
)

// Sample configuration structure
type SampleConfig struct {
	ProxyConfigs []SampleProxyConfig `yaml:"proxy_configs"`
}

type SampleProxyConfig struct {
	Name              string `yaml:"name"`
	KubernetesCluster string `yaml:"kubernetes_cluster"`
	RemoteHost        string `yaml:"remote_host"`
	LocalPort         int    `yaml:"local_port"`
	RemotePort        int    `yaml:"remote_port"`
}

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Generate a sample configuration file",
	Long: `Generate a sample configuration file that can be used to pre-populate proxy configurations.

The config file will be created in YAML format and can be customized to include your 
specific proxy configurations. By default, it will be created as 'aproxymate.yaml' 
in your home directory.`,
}

// initCmd represents the config init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a sample configuration file",
	Long: `Create a sample configuration file with example proxy configurations.

This command will create a 'aproxymate.yaml' file in your home directory (or the path 
specified with --output) with sample proxy configurations that you can customize.`,
	Run: func(cmd *cobra.Command, args []string) {
		output, _ := cmd.Flags().GetString("output")
		force, _ := cmd.Flags().GetBool("force")

		log.Info("Initializing configuration file", "output", output, "force", force)

		if output == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				log.Error("Failed to get home directory", "error", err)
				fmt.Printf("Error getting home directory: %v\n", err)
				os.Exit(1)
			}
			output = filepath.Join(home, "aproxymate.yaml")
		}

		// Check if file exists and force flag is not set
		if _, err := os.Stat(output); err == nil && !force {
			log.Warn("Configuration file already exists, not overwriting", "file", output)
			fmt.Printf("Config file already exists at %s. Use --force to overwrite.\n", output)
			os.Exit(1)
		}

		// Create sample config
		sampleConfig := SampleConfig{
			ProxyConfigs: []SampleProxyConfig{
				{
					Name:              "PostgreSQL Production",
					KubernetesCluster: "prod-cluster",
					RemoteHost:        "postgres-service",
					LocalPort:         5432,
					RemotePort:        5432,
				},
				{
					Name:              "Redis Staging",
					KubernetesCluster: "staging-cluster",
					RemoteHost:        "redis-service",
					LocalPort:         6379,
					RemotePort:        6379,
				},
				{
					Name:              "MySQL Development",
					KubernetesCluster: "dev-cluster",
					RemoteHost:        "mysql-service",
					LocalPort:         3306,
					RemotePort:        3306,
				},
			},
		}

		// Write to file
		data, err := yaml.Marshal(&sampleConfig)
		if err != nil {
			log.Error("Failed to marshal configuration", "error", err)
			fmt.Printf("Error marshaling config: %v\n", err)
			os.Exit(1)
		}

		if err := os.WriteFile(output, data, 0644); err != nil {
			log.Error("Failed to write configuration file", "file", output, "error", err)
			fmt.Printf("Error writing config file: %v\n", err)
			os.Exit(1)
		}

		log.Info("Sample configuration file created successfully", "file", output)
		fmt.Printf("Sample configuration file created at: %s\n", output)
		fmt.Println("\nYou can now customize this file and use it with:")
		fmt.Printf("  aproxymate gui --config %s\n", output)
	},
}

// showCmd represents the config show command
var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the current configuration file location and status",
	Long: `Display information about the current configuration file including:
- The location of the configuration file being used
- Whether a configuration file was found and loaded
- Basic statistics about the configuration`,
	Run: func(cmd *cobra.Command, args []string) {
		// Ensure viper is properly initialized and attempts to read config
		// This is needed because config show might be run without other commands that trigger config loading
		if viper.ConfigFileUsed() == "" {
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
							break
						}
					}
				}
			}
		}
		
		configFile := viper.ConfigFileUsed()

		if configFile == "" {
			fmt.Println("No configuration file is currently loaded.")
			fmt.Println("\nConfiguration search paths:")

			// Show where it would look for config files
			home, err := os.UserHomeDir()
			if err == nil {
				fmt.Printf("  %s/aproxymate.yaml\n", home)
				fmt.Printf("  %s/.aproxymate.yaml\n", home)
			}
			fmt.Println("  ./aproxymate.yaml")
			fmt.Println("  ./.aproxymate.yaml")

			fmt.Println("\nTo create a sample configuration file, run:")
			fmt.Println("  aproxymate config init")
			return
		}

		// Convert to absolute path for display
		absPath, err := filepath.Abs(configFile)
		if err != nil {
			absPath = configFile
		}

		fmt.Printf("Configuration file: %s\n", absPath)

		// Check if file exists and is readable
		if _, err := os.Stat(configFile); err != nil {
			log.Error("Configuration file not accessible", "file", configFile, "error", err)
			fmt.Printf("Status: ERROR - File not accessible (%v)\n", err)
			return
		}

		// First validate the raw YAML
		yamlData, err := os.ReadFile(configFile)
		if err != nil {
			log.Error("Failed to read configuration file", "file", configFile, "error", err)
			fmt.Printf("Status: ERROR - Failed to read file (%v)\n", err)
			return
		}

		// Validate YAML structure
		if err := lib.ValidateConfigYAML(yamlData); err != nil {
			log.Error("Configuration validation failed", "file", configFile, "error", err)
			fmt.Printf("Status: ERROR - Configuration validation failed (%v)\n", err)
			return
		}

		// Try to load and parse the config
		var config lib.AppConfig
		if err := viper.Unmarshal(&config); err != nil {
			log.Error("Failed to parse configuration", "file", configFile, "error", err)
			fmt.Printf("Status: ERROR - Failed to parse configuration (%v)\n", err)
			return
		}

		log.Info("Configuration validation successful", "file", configFile, "proxy_configs", len(config.ProxyConfigs))

		fmt.Printf("Status: OK - Configuration loaded and validated successfully\n")

		fmt.Printf("Proxy configurations: %d\n", len(config.ProxyConfigs))

		if len(config.ProxyConfigs) > 0 {
			fmt.Println("\nConfiguration summary:")
			clusterCounts := make(map[string]int)
			for _, proxy := range config.ProxyConfigs {
				clusterCounts[proxy.KubernetesCluster]++
			}

			for cluster, count := range clusterCounts {
				if cluster == "" {
					fmt.Printf("  (no cluster specified): %d proxy(s)\n", count)
				} else {
					fmt.Printf("  %s: %d proxy(s)\n", cluster, count)
				}
			}
		}
	},
}

// configListCmd represents the config list command
var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all proxy configurations from the config file",
	Long: `Display all proxy configurations defined in the current configuration file.

This command shows detailed information about each proxy configuration including:
- Name and description
- Kubernetes cluster
- Remote host and port
- Local port mapping`,
	Run: func(cmd *cobra.Command, args []string) {
		// Ensure viper is properly initialized and attempts to read config
		if viper.ConfigFileUsed() == "" {
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
							break
						}
					}
				}
			}
		}
		
		configFile := viper.ConfigFileUsed()

		if configFile == "" {
			fmt.Println("No configuration file is currently loaded.")
			fmt.Println("\nTo create a sample configuration file, run:")
			fmt.Println("  aproxymate config init")
			return
		}

		// Try to load and parse the config
		var config lib.AppConfig
		if err := viper.Unmarshal(&config); err != nil {
			log.Error("Failed to parse configuration for listing", "file", configFile, "error", err)
			fmt.Printf("Error parsing configuration file: %v\n", err)
			return
		}

		if len(config.ProxyConfigs) == 0 {
			fmt.Println("No proxy configurations found in the config file.")
			fmt.Println("\nTo add configurations, you can:")
			fmt.Println("  1. Edit the config file manually")
			fmt.Println("  2. Use the GUI to create and save configurations")
			fmt.Printf("  3. Run: aproxymate gui --config %s\n", configFile)
			return
		}

		fmt.Printf("Found %d proxy configuration(s) in %s:\n\n", len(config.ProxyConfigs), configFile)

		for i, proxy := range config.ProxyConfigs {
			fmt.Printf("%d. %s\n", i+1, proxy.Name)
			fmt.Printf("   Cluster: %s\n", proxy.KubernetesCluster)
			fmt.Printf("   Remote:  %s:%d\n", proxy.RemoteHost, proxy.RemotePort)
			fmt.Printf("   Local:   localhost:%d\n", proxy.LocalPort)

			if i < len(config.ProxyConfigs)-1 {
				fmt.Println()
			}
		}

		fmt.Printf("\nTo start the GUI with these configurations, run:\n")
		fmt.Printf("  aproxymate gui --config %s\n", configFile)
	},
}

func init() {
	configCmd.AddCommand(initCmd)
	configCmd.AddCommand(showCmd)
	configCmd.AddCommand(configListCmd)
	rootCmd.AddCommand(configCmd)

	// Add flags for the config init command
	initCmd.Flags().StringP("output", "o", "", "Output path for the config file (default: $HOME/aproxymate.yaml)")
	initCmd.Flags().BoolP("force", "f", false, "Force overwrite existing config file")
}
