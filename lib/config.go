package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// ProxyConfig represents a single proxy configuration
type ProxyConfig struct {
	Name              string `json:"name" mapstructure:"name" yaml:"name"`
	KubernetesCluster string `json:"kubernetes_cluster" mapstructure:"kubernetes_cluster" yaml:"kubernetes_cluster"`
	RemoteHost        string `json:"remote_host" mapstructure:"remote_host" yaml:"remote_host"`
	LocalPort         int    `json:"local_port" mapstructure:"local_port" yaml:"local_port"`
	RemotePort        int    `json:"remote_port" mapstructure:"remote_port" yaml:"remote_port"`
}

// AppConfig represents the main application configuration
type AppConfig struct {
	ProxyConfigs []ProxyConfig `json:"proxy_configs" mapstructure:"proxy_configs" yaml:"proxy_configs"`
}

// ValidateConfigYAML attempts to unmarshal YAML data to our config struct and returns any errors
func ValidateConfigYAML(yamlData []byte) error {
	var config AppConfig
	if err := yaml.Unmarshal(yamlData, &config); err != nil {
		return fmt.Errorf("YAML structure error: %w", err)
	}

	// Basic validation after successful unmarshal
	if len(config.ProxyConfigs) == 0 {
		return fmt.Errorf("no proxy configurations found in config file")
	}

	// Validate each proxy config
	for i, proxy := range config.ProxyConfigs {
		if proxy.Name == "" {
			return fmt.Errorf("proxy config #%d is missing 'name' field", i+1)
		}
		// Note: kubernetes_cluster can be empty - we'll prompt for it if needed
		if proxy.RemoteHost == "" {
			return fmt.Errorf("proxy config #%d (%s) is missing 'remote_host' field", i+1, proxy.Name)
		}
		if proxy.LocalPort <= 0 || proxy.LocalPort > 65535 {
			return fmt.Errorf("proxy config #%d (%s) has invalid 'local_port': %d (must be 1-65535)", i+1, proxy.Name, proxy.LocalPort)
		}
		if proxy.RemotePort <= 0 || proxy.RemotePort > 65535 {
			return fmt.Errorf("proxy config #%d (%s) has invalid 'remote_port': %d (must be 1-65535)", i+1, proxy.Name, proxy.RemotePort)
		}
	}

	return nil
}

// EnsureUniqueLocalPorts ensures all proxy configurations have unique local ports
func EnsureUniqueLocalPorts(configs []ProxyConfig) []ProxyConfig {
	if len(configs) <= 1 {
		return configs
	}

	// Create a copy to avoid modifying the original slice
	result := make([]ProxyConfig, len(configs))
	copy(result, configs)

	// Sort by local port to process in order
	sort.Slice(result, func(i, j int) bool {
		return result[i].LocalPort < result[j].LocalPort
	})

	usedPorts := make(map[int]bool)

	for i := range result {
		originalPort := result[i].LocalPort

		// Find next available port if current port is already used
		if usedPorts[originalPort] {
			result[i].LocalPort = findNextAvailablePortFromSet(usedPorts, originalPort)
		}

		usedPorts[result[i].LocalPort] = true
	}

	return result
}

// findNextAvailablePortFromSet finds the next available port from a set of used ports
func findNextAvailablePortFromSet(usedPorts map[int]bool, startPort int) int {
	// Start from the provided port
	port := startPort
	for {
		if !usedPorts[port] && port >= 1024 && port <= 65535 {
			return port
		}
		port++

		// If we've gone beyond the valid range, start from a reasonable default
		if port > 65535 {
			port = 3000
		}

		// Prevent infinite loop - check if we've circled back
		if port == startPort {
			break
		}
	}

	// Fallback: find any available port in the range 3000-9999
	for port := 3000; port <= 9999; port++ {
		if !usedPorts[port] {
			return port
		}
	}

	// Final fallback: use the original port
	return startPort
}

// GetUsedLocalPorts returns a set of all local ports currently in use
func GetUsedLocalPorts(configs []ProxyConfig) map[int]bool {
	usedPorts := make(map[int]bool)
	for _, config := range configs {
		usedPorts[config.LocalPort] = true
	}
	return usedPorts
}

// GetNextAvailablePort finds the next available local port starting from the given port
func GetNextAvailablePort(configs []ProxyConfig, startPort int) int {
	usedPorts := GetUsedLocalPorts(configs)
	return findNextAvailablePortFromSet(usedPorts, startPort)
}

// ValidateUniqueLocalPorts checks if all local ports in the configuration are unique
func ValidateUniqueLocalPorts(configs []ProxyConfig) error {
	portCounts := make(map[int][]string)

	for _, config := range configs {
		portCounts[config.LocalPort] = append(portCounts[config.LocalPort], config.Name)
	}

	var conflicts []string
	for port, names := range portCounts {
		if len(names) > 1 {
			conflicts = append(conflicts, fmt.Sprintf("port %d used by: %v", port, names))
		}
	}

	if len(conflicts) > 0 {
		return fmt.Errorf("local port conflicts detected: %v", conflicts)
	}

	return nil
}

// FindConfigsWithMissingClusters returns a list of proxy configs that don't have a kubernetes_cluster specified
func FindConfigsWithMissingClusters(configs []ProxyConfig) []ProxyConfig {
	var missingClusterConfigs []ProxyConfig

	for _, config := range configs {
		if config.KubernetesCluster == "" {
			missingClusterConfigs = append(missingClusterConfigs, config)
		}
	}

	return missingClusterConfigs
}

// UpdateConfigsWithCluster updates all configurations with missing clusters to use the specified cluster
func UpdateConfigsWithCluster(configs []ProxyConfig, clusterName string) []ProxyConfig {
	updatedConfigs := make([]ProxyConfig, len(configs))
	copy(updatedConfigs, configs)

	for i := range updatedConfigs {
		if updatedConfigs[i].KubernetesCluster == "" {
			updatedConfigs[i].KubernetesCluster = clusterName
		}
	}

	return updatedConfigs
}

// HasConfigsWithMissingClusters checks if any proxy configs are missing cluster specifications
func HasConfigsWithMissingClusters(configs []ProxyConfig) bool {
	for _, config := range configs {
		if config.KubernetesCluster == "" {
			return true
		}
	}
	return false
}

// GetDefaultConfigPaths returns standard config file locations
func GetDefaultConfigPaths() []string {
	return GetConfigSearchPaths()
}

// GetAbsolutePathForDisplay converts path to absolute for consistent display
func GetAbsolutePathForDisplay(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return path // fallback to original path
	}
	return absPath
}

// FindAndLoadConfigFile searches standard locations and loads config
func FindAndLoadConfigFile() (string, error) {
	// If viper already has a config file, use it
	if configFile := viper.ConfigFileUsed(); configFile != "" {
		return configFile, nil
	}

	// Search in standard locations
	configPaths := GetDefaultConfigPaths()

	for _, path := range configPaths {
		if _, err := os.Stat(path); err == nil {
			// Found a config file, set it in viper
			viper.SetConfigFile(path)
			if err := viper.ReadInConfig(); err == nil {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("no configuration file found in standard locations")
}

// EnsureConfigLoaded ensures a config file is loaded in viper
func EnsureConfigLoaded() error {
	_, err := FindAndLoadConfigFile()
	return err
}
