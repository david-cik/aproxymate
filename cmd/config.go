package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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

		log.Debug("Initializing configuration file", "output", output, "force", force)

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

		log.Debug("Sample configuration file created successfully", "file", output)
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

		log.Debug("Configuration validation successful", "file", configFile, "proxy_configs", len(config.ProxyConfigs))

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

// configFixCmd represents the config fix command
var configFixCmd = &cobra.Command{
	Use:   "fix",
	Short: "Fix configuration issues like missing Kubernetes clusters",
	Long: `Check the configuration file for common issues and fix them interactively.

This command will:
- Check for proxy configurations missing kubernetes_cluster fields
- Prompt you to select a cluster for configurations that need one
- Update and save the configuration file with the fixes

Example:
  aproxymate config fix
  aproxymate config fix --config ./my-config.yaml`,
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

		// Convert to absolute path for display
		absPath, err := filepath.Abs(configFile)
		if err != nil {
			absPath = configFile
		}

		fmt.Printf("Checking configuration file: %s\n", absPath)

		// Try to load and parse the config
		var config lib.AppConfig
		if err := viper.Unmarshal(&config); err != nil {
			log.Error("Failed to parse configuration for fixing", "file", configFile, "error", err)
			fmt.Printf("Error parsing configuration file: %v\n", err)
			return
		}

		if len(config.ProxyConfigs) == 0 {
			fmt.Println("No proxy configurations found in the config file.")
			return
		}

		fmt.Printf("Found %d proxy configuration(s)\n", len(config.ProxyConfigs))

		// Check for missing clusters
		missingClusterConfigs := lib.FindConfigsWithMissingClusters(config.ProxyConfigs)

		if len(missingClusterConfigs) == 0 {
			fmt.Println("✅ All configurations have Kubernetes clusters specified. No fixes needed.")
			return
		}

		fmt.Printf("\n⚠️  Found %d configuration(s) missing Kubernetes cluster:\n", len(missingClusterConfigs))
		for i, proxyConfig := range missingClusterConfigs {
			fmt.Printf("  %d. %s (%s:%d)\n", i+1, proxyConfig.Name, proxyConfig.RemoteHost, proxyConfig.RemotePort)
		}

		// Prompt for cluster selection
		selectedCluster, err := lib.PromptForKubernetesCluster()
		if err != nil {
			log.Error("Failed to select Kubernetes cluster", "error", err)
			fmt.Printf("Error selecting cluster: %v\n", err)
			os.Exit(1)
		}

		// Update configurations with the selected cluster
		updatedConfigs := lib.UpdateConfigsWithCluster(config.ProxyConfigs, selectedCluster)

		// Save the updated configuration
		finalConfig := lib.AppConfig{
			ProxyConfigs: updatedConfigs,
		}

		data, err := yaml.Marshal(&finalConfig)
		if err != nil {
			log.Error("Failed to marshal updated configuration", "error", err)
			fmt.Printf("Error marshaling config: %v\n", err)
			os.Exit(1)
		}

		if err := os.WriteFile(configFile, data, 0644); err != nil {
			log.Error("Failed to write updated configuration file", "file", configFile, "error", err)
			fmt.Printf("Error writing config file: %v\n", err)
			os.Exit(1)
		}

		log.Debug("Configuration fixed successfully",
			"file", absPath,
			"cluster", selectedCluster,
			"fixed_configs", len(missingClusterConfigs))

		fmt.Printf("\n✅ Configuration fixed successfully!\n")
		fmt.Printf("Updated %d configuration(s) with cluster: %s\n", len(missingClusterConfigs), selectedCluster)
		fmt.Printf("Configuration saved to: %s\n", absPath)
		fmt.Println("\nTo start the GUI with the fixed configuration:")
		fmt.Printf("  aproxymate gui --config %s\n", absPath)
	},
}
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

// rdsImportCmd represents the config rds-import command
var rdsImportCmd = &cobra.Command{
	Use:   "rds-import",
	Short: "Import RDS endpoints from AWS and merge into configuration",
	Long: `Import RDS endpoints from your AWS account and merge them into your aproxymate configuration.

This command will:
- Connect to AWS using your configured credentials and specified profile
- Discover all RDS instances and clusters in the specified region
- Generate proxy configurations for each endpoint
- Assign unique local ports automatically
- Merge the new configurations with your existing ones

The command requires:
- AWS profile (specify via --profile flag or AWS_PROFILE environment variable)
- AWS region (specify via --region flag or AWS_REGION environment variable)
- AWS credentials configured for the specified profile via:
  - AWS CLI (aws configure --profile <profile-name>)
  - Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
  - IAM roles (if running on EC2)
  - AWS credentials file (~/.aws/credentials)

Examples:
  aproxymate config rds-import --cluster eks-prod --region us-west-2 --profile production
  aproxymate config rds-import --cluster eks-prod --region us-east-1 --profile my-profile
  aproxymate config rds-import --cluster eks-prod --region eu-west-1 --starting-port 4000 --profile dev
  aproxymate config rds-import --cluster eks-prod --region us-west-2 --engines mysql,postgres --profile prod
  
  # Using environment variables:
  export AWS_PROFILE=production
  export AWS_REGION=us-west-2
  aproxymate config rds-import --cluster eks-prod`,
	Run: func(cmd *cobra.Command, args []string) {
		cluster, _ := cmd.Flags().GetString("cluster")
		region, _ := cmd.Flags().GetString("region")
		profile, _ := cmd.Flags().GetString("profile")
		startingPort, _ := cmd.Flags().GetInt("starting-port")
		enginesFlag, _ := cmd.Flags().GetString("engines")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		output, _ := cmd.Flags().GetString("output")

		// Get AWS profile from environment if not specified on command line
		if profile == "" {
			profile = os.Getenv("AWS_PROFILE")
		}

		// Get AWS region from environment if not specified on command line
		if region == "" {
			region = os.Getenv("AWS_REGION")
		}

		log.Debug("Starting AWS RDS endpoint import",
			"cluster", cluster,
			"region", region,
			"profile", profile,
			"starting_port", startingPort,
			"engines", enginesFlag,
			"dry_run", dryRun)

		if cluster == "" {
			fmt.Println("Error: --cluster flag is required")
			fmt.Println("\nThe cluster flag specifies which Kubernetes cluster the RDS endpoints should be configured for.")
			fmt.Println("Example: aproxymate config rds-import --cluster eks-prod")
			os.Exit(1)
		}

		if profile == "" {
			fmt.Println("Error: AWS profile is required")
			fmt.Println("\nPlease specify an AWS profile using one of these methods:")
			fmt.Println("  - Command line flag: --profile my-profile")
			fmt.Println("  - Environment variable: export AWS_PROFILE=my-profile")
			fmt.Println("\nThis ensures you're using the correct AWS account and prevents accidental use of default credentials.")
			os.Exit(1)
		}

		if region == "" {
			fmt.Println("Error: AWS region is required")
			fmt.Println("\nPlease specify an AWS region using one of these methods:")
			fmt.Println("  - Command line flag: --region us-west-2")
			fmt.Println("  - Environment variable: export AWS_REGION=us-west-2")
			fmt.Println("\nThis ensures you're targeting the correct AWS region.")
			os.Exit(1)
		}

		// Parse engines filter
		var engines []string
		if enginesFlag != "" {
			engines = strings.Split(strings.ReplaceAll(enginesFlag, " ", ""), ",")
		}

		// Create AWS config
		awsConfig := lib.AWSConfig{
			Region:  region,
			Profile: profile,
		}

		// Validate AWS credentials
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		fmt.Printf("Validating AWS credentials (region: %s, profile: %s)...\n", awsConfig.Region, awsConfig.Profile)

		if err := lib.ValidateAWSCredentials(ctx, awsConfig); err != nil {
			log.Error("AWS credentials validation failed", "error", err)
			fmt.Printf("AWS credentials validation failed: %v\n", err)
			fmt.Println("\nPlease ensure:")
			fmt.Println("  1. AWS profile is specified via --profile flag or AWS_PROFILE environment variable")
			fmt.Println("  2. AWS region is specified via --region flag or AWS_REGION environment variable")
			fmt.Println("  3. AWS credentials are configured for the specified profile via:")
			fmt.Println("     - AWS CLI: aws configure --profile <profile-name>")
			fmt.Println("     - Environment variables: AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY")
			fmt.Println("     - IAM roles (if running on EC2)")
			fmt.Println("     - AWS credentials file in ~/.aws/credentials")
			os.Exit(1)
		}

		fmt.Println("AWS credentials validated successfully")

		// Fetch RDS endpoints
		fmt.Println("Discovering RDS endpoints...")
		endpoints, err := lib.GetAWSRDSEndpoints(ctx, awsConfig)
		if err != nil {
			log.Error("Failed to fetch RDS endpoints", "error", err)
			fmt.Printf("Failed to fetch RDS endpoints: %v\n", err)
			os.Exit(1)
		}

		if len(endpoints) == 0 {
			fmt.Printf("No RDS endpoints found in region %s", awsConfig.Region)
			if awsConfig.Profile != "" {
				fmt.Printf(" (profile: %s)", awsConfig.Profile)
			}
			fmt.Println()
			fmt.Println("\nThis could mean:")
			fmt.Println("  - No RDS instances/clusters exist in this region")
			fmt.Println("  - Your credentials don't have permission to list RDS resources")
			fmt.Println("  - You're looking in the wrong region")
			return
		}

		fmt.Printf("Found %d RDS endpoints\n", len(endpoints))

		// Filter by engines if specified
		if len(engines) > 0 {
			endpoints = lib.FilterRDSEndpointsByEngine(endpoints, engines)
			fmt.Printf("Filtered to %d endpoints matching engines: %s\n", len(endpoints), strings.Join(engines, ", "))
		}

		// Filter by status (only available/running)
		endpoints = lib.FilterRDSEndpointsByStatus(endpoints, []string{"available", "running"})
		fmt.Printf("Filtered to %d available endpoints\n", len(endpoints))

		if len(endpoints) == 0 {
			fmt.Println("No available RDS endpoints found after filtering")
			return
		}

		// Load existing configuration
		var existingConfig lib.AppConfig
		configFile := output
		if configFile == "" {
			// Try to find existing config file
			if viper.ConfigFileUsed() != "" {
				configFile = viper.ConfigFileUsed()
			} else {
				// Use default location
				home, err := os.UserHomeDir()
				if err != nil {
					log.Error("Failed to get home directory", "error", err)
					fmt.Printf("Error getting home directory: %v\n", err)
					os.Exit(1)
				}
				configFile = filepath.Join(home, "aproxymate.yaml")
			}
		}

		// Try to load existing configuration
		if _, err := os.Stat(configFile); err == nil {
			yamlData, err := os.ReadFile(configFile)
			if err != nil {
				log.Error("Failed to read existing configuration", "file", configFile, "error", err)
				fmt.Printf("Error reading existing config file: %v\n", err)
				os.Exit(1)
			}

			if err := yaml.Unmarshal(yamlData, &existingConfig); err != nil {
				log.Error("Failed to parse existing configuration", "file", configFile, "error", err)
				fmt.Printf("Error parsing existing config file: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("Loaded existing configuration with %d proxy configs\n", len(existingConfig.ProxyConfigs))
		} else {
			fmt.Println("No existing configuration found, creating new one")
		}

		// Determine starting port
		if startingPort == 0 {
			startingPort = lib.GetStartingPortForAWSConfigs(existingConfig.ProxyConfigs)
		}

		// Convert RDS endpoints to proxy configs
		newConfigs := lib.ConvertRDSEndpointsToProxyConfigs(endpoints, cluster, startingPort)
		fmt.Printf("Generated %d proxy configurations\n", len(newConfigs))

		// Merge configurations
		mergedConfigs := lib.MergeProxyConfigs(existingConfig.ProxyConfigs, newConfigs)
		newConfigsAdded := len(mergedConfigs) - len(existingConfig.ProxyConfigs)

		if dryRun {
			fmt.Println("DRY RUN MODE - Changes will not be saved")
		}

		fmt.Printf("Configuration summary:\n")
		fmt.Printf("  Existing configurations: %d\n", len(existingConfig.ProxyConfigs))
		fmt.Printf("  New configurations added: %d\n", newConfigsAdded)
		fmt.Printf("  Total configurations: %d\n", len(mergedConfigs))

		if newConfigsAdded > 0 {
			fmt.Println("\nNew configurations that will be added:")
			addedCount := 0
			for _, config := range mergedConfigs {
				// Check if this is a new config
				isNew := true
				for _, existing := range existingConfig.ProxyConfigs {
					if existing.RemoteHost == config.RemoteHost && existing.RemotePort == config.RemotePort {
						isNew = false
						break
					}
				}
				if isNew {
					addedCount++
					fmt.Printf("  %d. %s\n", addedCount, config.Name)
					fmt.Printf("     Cluster: %s\n", config.KubernetesCluster)
					fmt.Printf("     Remote:  %s:%d\n", config.RemoteHost, config.RemotePort)
					fmt.Printf("     Local:   localhost:%d\n", config.LocalPort)
					fmt.Println()
				}
			}
		}

		if dryRun {
			fmt.Println("Dry run completed. Use --dry-run=false to save changes.")
			return
		}

		if newConfigsAdded == 0 {
			fmt.Println("No new configurations to add - all RDS endpoints are already configured")
			return
		}

		// Save the merged configuration
		finalConfig := lib.AppConfig{
			ProxyConfigs: mergedConfigs,
		}

		data, err := yaml.Marshal(&finalConfig)
		if err != nil {
			log.Error("Failed to marshal configuration", "error", err)
			fmt.Printf("Error marshaling config: %v\n", err)
			os.Exit(1)
		}

		if err := os.WriteFile(configFile, data, 0644); err != nil {
			log.Error("Failed to write configuration file", "file", configFile, "error", err)
			fmt.Printf("Error writing config file: %v\n", err)
			os.Exit(1)
		}

		// Convert to absolute path for display
		absPath, err := filepath.Abs(configFile)
		if err != nil {
			absPath = configFile
		}

		log.Debug("AWS RDS import completed successfully",
			"file", absPath,
			"total_configs", len(mergedConfigs),
			"new_configs", newConfigsAdded)

		fmt.Printf("Configuration saved to: %s\n", absPath)
		fmt.Printf("Total configurations: %d (%d new)\n", len(mergedConfigs), newConfigsAdded)
		fmt.Println("\nTo start the GUI with these configurations:")
		fmt.Printf("  aproxymate gui --config %s\n", absPath)
	},
}

func init() {
	configCmd.AddCommand(initCmd)
	configCmd.AddCommand(showCmd)
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configFixCmd)
	configCmd.AddCommand(rdsImportCmd)
	rootCmd.AddCommand(configCmd)

	// Add flags for the config init command
	initCmd.Flags().StringP("output", "o", "", "Output path for the config file (default: $HOME/aproxymate.yaml)")
	initCmd.Flags().BoolP("force", "f", false, "Force overwrite existing config file")

	// Add flags for the config rds-import command
	rdsImportCmd.Flags().StringP("cluster", "c", "", "Kubernetes cluster name to associate with RDS endpoints (required)")
	rdsImportCmd.Flags().StringP("region", "r", "", "AWS region (required - can also be set via AWS_REGION environment variable)")
	rdsImportCmd.Flags().StringP("profile", "p", "", "AWS profile to use (required - can also be set via AWS_PROFILE environment variable)")
	rdsImportCmd.Flags().IntP("starting-port", "s", 0, "Starting local port number (defaults to next available port)")
	rdsImportCmd.Flags().StringP("engines", "e", "", "Comma-separated list of database engines to include (e.g., mysql,postgres)")
	rdsImportCmd.Flags().Bool("dry-run", false, "Show what would be imported without making changes")
	rdsImportCmd.Flags().StringP("output", "o", "", "Output path for the config file (defaults to existing config or ~/aproxymate.yaml)")

	// Mark cluster as required for rds-import command
	rdsImportCmd.MarkFlagRequired("cluster")
}
