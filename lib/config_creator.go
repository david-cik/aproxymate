package lib

import (
	"fmt"
	"os"
	"path/filepath"
)

// GetConfigLocations returns the available configuration file locations
func GetConfigLocations() []ConfigLocation {
	home, _ := os.UserHomeDir()

	// Use shared config paths but convert to ConfigLocation format
	configPaths := GetDefaultConfigPaths()
	locations := make([]ConfigLocation, 0, len(configPaths))

	for _, path := range configPaths {
		var displayName, description string

		if filepath.Dir(path) == "." {
			// Current directory
			if filepath.Base(path) == "aproxymate.yaml" {
				displayName = "Current directory"
			} else {
				displayName = "Current directory (hidden)"
			}
			description = path
		} else {
			// Home directory
			if filepath.Base(path) == "aproxymate.yaml" {
				displayName = "Home directory"
			} else {
				displayName = "Home directory (hidden)"
			}
			description = fmt.Sprintf("%s/%s", home, filepath.Base(path))
		}

		locations = append(locations, ConfigLocation{
			Path:        path,
			DisplayName: displayName,
			Description: description,
		})
	}

	return locations
}

// CreateSampleConfigFile creates a sample configuration file at the specified location
func CreateSampleConfigFile(location string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(location)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Sample configuration content
	sampleConfig := `# Aproxymate Configuration File
# This file defines proxy configurations for Kubernetes port forwarding
#
# Each proxy configuration creates a local port that forwards traffic
# to a remote service running in a Kubernetes cluster.

proxy_configs:
  # Example: PostgreSQL Database
  - name: "PostgreSQL Production"
    # The Kubernetes context/cluster name (must match your kubeconfig)
    kubernetes_cluster: "prod-cluster"
    # The service name or pod name to connect to
    remote_host: "postgres-service"
    # The port to bind locally (where you'll connect)
    local_port: 5432
    # The port on the remote service/pod
    remote_port: 5432

  # Example: Redis Cache
  - name: "Redis Staging"
    kubernetes_cluster: "staging-cluster"
    remote_host: "redis-service"
    local_port: 6379
    remote_port: 6379

  # Example: API Service
  - name: "API Development"
    kubernetes_cluster: "dev-cluster"
    remote_host: "api-service"
    local_port: 8080
    remote_port: 8080

# Configuration Tips:
# 1. The 'kubernetes_cluster' should match a context name in your kubeconfig
#    Run 'kubectl config get-contexts' to see available contexts
#
# 2. The 'remote_host' can be:
#    - A service name (e.g., "postgres-service")
#    - A pod name (e.g., "postgres-pod-12345")
#    - A service with namespace (e.g., "postgres-service.database")
#
# 3. Choose unique 'local_port' values to avoid conflicts
#
# 4. The GUI allows you to start/stop these proxy connections easily
#
# To edit this file, you can:
# - Edit it manually in your favorite text editor
# - Use the GUI interface to modify and save configurations
# - Run 'aproxymate config list' to see all configurations
# - Run 'aproxymate config validate' to check for errors
`

	// Write the sample configuration
	if err := os.WriteFile(location, []byte(sampleConfig), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
