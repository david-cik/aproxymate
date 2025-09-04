package lib

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"

	log "aproxymate/lib/logger"
)

// AWSConfig represents AWS configuration options
type AWSConfig struct {
	Region  string
	Profile string
}

// RDSEndpoint represents an RDS endpoint discovered from AWS
type RDSEndpoint struct {
	Identifier  string
	Endpoint    string
	Port        int32
	Engine      string
	Status      string
	IsCluster   bool
	ClusterRole string // primary, reader, writer, etc.
}

// GetAWSRDSEndpoints fetches all RDS endpoints from the specified AWS account/region
func GetAWSRDSEndpoints(ctx context.Context, awsConfig AWSConfig) ([]RDSEndpoint, error) {
	opCtx, _ := log.StartOperation(ctx, "aws", "fetch_rds_endpoints")
	defer opCtx.Complete("fetch_rds_endpoints", nil)

	opCtx.Debug("Fetching RDS endpoints from AWS", "region", awsConfig.Region, "profile", awsConfig.Profile)

	// AWS profile is now required
	if awsConfig.Profile == "" {
		return nil, fmt.Errorf("AWS profile is required. Please specify a profile using --profile flag or set AWS_PROFILE environment variable")
	}

	// AWS region is now required
	if awsConfig.Region == "" {
		return nil, fmt.Errorf("AWS region is required. Please specify a region using --region flag or set AWS_REGION environment variable")
	}

	// Load AWS config
	var cfg aws.Config
	var err error

	configOptions := []func(*config.LoadOptions) error{
		config.WithRegion(awsConfig.Region),
		config.WithSharedConfigProfile(awsConfig.Profile),
	}

	cfg, err = config.LoadDefaultConfig(ctx, configOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config with profile '%s': %w", awsConfig.Profile, err)
	}

	rdsClient := rds.NewFromConfig(cfg)

	var endpoints []RDSEndpoint

	// Get RDS instances
	instances, err := getAllRDSInstances(ctx, rdsClient)
	if err != nil {
		opCtx.Error("Failed to fetch RDS instances", err)
		log.LogAWSOperation("describe_db_instances", awsConfig.Region, awsConfig.Profile, err)
		return nil, fmt.Errorf("failed to fetch RDS instances: %w", err)
	}

	// Only add standalone instances (not part of a cluster)
	for _, instance := range instances {
		// Skip instances that are part of a cluster - we'll handle clusters separately
		if instance.DBClusterIdentifier != nil {
			continue
		}

		endpoint := RDSEndpoint{
			Identifier:  aws.ToString(instance.DBInstanceIdentifier),
			Endpoint:    aws.ToString(instance.Endpoint.Address),
			Port:        aws.ToInt32(instance.Endpoint.Port),
			Engine:      aws.ToString(instance.Engine),
			Status:      aws.ToString(instance.DBInstanceStatus),
			IsCluster:   false,
			ClusterRole: "",
		}
		endpoints = append(endpoints, endpoint)
	}

	// Get RDS clusters
	clusters, err := getAllRDSClusters(ctx, rdsClient)
	if err != nil {
		opCtx.Error("Failed to fetch RDS clusters", err)
		log.LogAWSOperation("describe_db_clusters", awsConfig.Region, awsConfig.Profile, err)
		return nil, fmt.Errorf("failed to fetch RDS clusters: %w", err)
	}

	// Only add the primary (writer) endpoint for each cluster
	for _, cluster := range clusters {
		if cluster.Endpoint != nil && aws.ToString(cluster.Endpoint) != "" {
			endpoint := RDSEndpoint{
				Identifier:  aws.ToString(cluster.DBClusterIdentifier),
				Endpoint:    aws.ToString(cluster.Endpoint),
				Port:        aws.ToInt32(cluster.Port),
				Engine:      aws.ToString(cluster.Engine),
				Status:      aws.ToString(cluster.Status),
				IsCluster:   true,
				ClusterRole: "primary",
			}
			endpoints = append(endpoints, endpoint)
		}
	}

	opCtx.Debug("Successfully fetched RDS endpoints", "total_endpoints", len(endpoints))
	log.LogAWSOperation("fetch_rds_endpoints", awsConfig.Region, awsConfig.Profile, nil)
	return endpoints, nil
}

// getAllRDSInstances fetches all RDS instances using pagination
func getAllRDSInstances(ctx context.Context, client *rds.Client) ([]types.DBInstance, error) {
	var instances []types.DBInstance
	var marker *string

	for {
		input := &rds.DescribeDBInstancesInput{
			Marker: marker,
		}

		output, err := client.DescribeDBInstances(ctx, input)
		if err != nil {
			return nil, err
		}

		instances = append(instances, output.DBInstances...)

		if output.Marker == nil {
			break
		}
		marker = output.Marker
	}

	return instances, nil
}

// getAllRDSClusters fetches all RDS clusters using pagination
func getAllRDSClusters(ctx context.Context, client *rds.Client) ([]types.DBCluster, error) {
	var clusters []types.DBCluster
	var marker *string

	for {
		input := &rds.DescribeDBClustersInput{
			Marker: marker,
		}

		output, err := client.DescribeDBClusters(ctx, input)
		if err != nil {
			return nil, err
		}

		clusters = append(clusters, output.DBClusters...)

		if output.Marker == nil {
			break
		}
		marker = output.Marker
	}

	return clusters, nil
}

// ConvertRDSEndpointsToProxyConfigs converts RDS endpoints to ProxyConfig objects
func ConvertRDSEndpointsToProxyConfigs(endpoints []RDSEndpoint, kubernetesCluster string, startingPort int) []ProxyConfig {
	var configs []ProxyConfig
	currentPort := startingPort

	// Sort endpoints by identifier for consistent ordering
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].Identifier < endpoints[j].Identifier
	})

	for _, endpoint := range endpoints {
		// Skip endpoints that are not available/accessible
		if endpoint.Status != "available" && endpoint.Status != "running" {
			log.Debug("Skipping RDS endpoint with non-available status",
				"identifier", endpoint.Identifier,
				"status", endpoint.Status)
			continue
		}

		// Generate a meaningful name
		name := generateProxyConfigName(endpoint)

		config := ProxyConfig{
			Name:              name,
			KubernetesCluster: kubernetesCluster,
			RemoteHost:        endpoint.Endpoint,
			LocalPort:         currentPort,
			RemotePort:        int(endpoint.Port),
		}

		configs = append(configs, config)
		currentPort++
	}

	return configs
}

// generateProxyConfigName creates a meaningful name for the proxy configuration
func generateProxyConfigName(endpoint RDSEndpoint) string {
	var parts []string

	// Add identifier
	parts = append(parts, endpoint.Identifier)

	// Add cluster role if applicable (but skip "primary" as it's redundant)
	if endpoint.IsCluster && endpoint.ClusterRole != "" && endpoint.ClusterRole != "primary" {
		parts = append(parts, endpoint.ClusterRole)
	}

	// Add engine for context
	if endpoint.Engine != "" {
		parts = append(parts, strings.ToLower(endpoint.Engine))
	}

	name := strings.Join(parts, "-")

	// Add endpoint for uniqueness if needed
	if endpoint.Endpoint != "" {
		name = fmt.Sprintf("%s (%s)", name, endpoint.Endpoint)
	}

	return name
}

// MergeProxyConfigs merges new proxy configs with existing ones, ensuring unique local ports
func MergeProxyConfigs(existingConfigs []ProxyConfig, newConfigs []ProxyConfig) []ProxyConfig {
	log.Debug("Merging proxy configurations",
		"existing_count", len(existingConfigs),
		"new_count", len(newConfigs))

	// Create a map of existing configurations by remote host for deduplication
	existingByHost := make(map[string]ProxyConfig)
	usedPorts := make(map[int]bool)

	// Track existing configurations and used ports
	for _, config := range existingConfigs {
		key := fmt.Sprintf("%s:%d", config.RemoteHost, config.RemotePort)
		existingByHost[key] = config
		usedPorts[config.LocalPort] = true
	}

	var mergedConfigs []ProxyConfig
	mergedConfigs = append(mergedConfigs, existingConfigs...)

	// Add new configurations, ensuring unique local ports
	for _, newConfig := range newConfigs {
		key := fmt.Sprintf("%s:%d", newConfig.RemoteHost, newConfig.RemotePort)

		if existing, exists := existingByHost[key]; exists {
			log.Debug("Skipping duplicate RDS endpoint",
				"endpoint", newConfig.RemoteHost,
				"port", newConfig.RemotePort,
				"existing_name", existing.Name)
			continue
		}

		// Ensure unique local port
		newConfig.LocalPort = findNextAvailablePort(usedPorts, newConfig.LocalPort)
		usedPorts[newConfig.LocalPort] = true

		mergedConfigs = append(mergedConfigs, newConfig)
		log.Debug("Added new RDS endpoint configuration",
			"name", newConfig.Name,
			"endpoint", newConfig.RemoteHost,
			"local_port", newConfig.LocalPort,
			"remote_port", newConfig.RemotePort)
	}

	log.Debug("Configuration merge completed", "total_configs", len(mergedConfigs))
	return mergedConfigs
}

// findNextAvailablePort finds the next available port starting from the preferred port
func findNextAvailablePort(usedPorts map[int]bool, preferredPort int) int {
	// Start from the preferred port and find the next available one
	port := preferredPort
	for {
		if !usedPorts[port] && port >= 1024 && port <= 65535 {
			return port
		}
		port++

		// If we've gone beyond the valid range, start from a reasonable default
		if port > 65535 {
			port = 3000 // Start from 3000 as a reasonable default for databases
		}

		// Prevent infinite loop
		if port == preferredPort {
			break
		}
	}

	// Fallback: find any available port in the range 3000-9999
	for port := 3000; port <= 9999; port++ {
		if !usedPorts[port] {
			return port
		}
	}

	// Final fallback: use the preferred port even if it might conflict
	log.Warn("Could not find available port, using preferred port", "port", preferredPort)
	return preferredPort
}

// ValidateAWSCredentials checks if AWS credentials are properly configured
func ValidateAWSCredentials(ctx context.Context, awsConfig AWSConfig) error {
	log.Debug("Validating AWS credentials", "region", awsConfig.Region, "profile", awsConfig.Profile)

	// AWS profile is now required - check if it's provided
	if awsConfig.Profile == "" {
		return fmt.Errorf("AWS profile is required. Please specify a profile using --profile flag or set AWS_PROFILE environment variable")
	}

	// AWS region is now required - check if it's provided
	if awsConfig.Region == "" {
		return fmt.Errorf("AWS region is required. Please specify a region using --region flag or set AWS_REGION environment variable")
	}

	configOptions := []func(*config.LoadOptions) error{
		config.WithRegion(awsConfig.Region),
		config.WithSharedConfigProfile(awsConfig.Profile),
	}

	cfg, err := config.LoadDefaultConfig(ctx, configOptions...)
	if err != nil {
		return fmt.Errorf("failed to load AWS config with profile '%s': %w", awsConfig.Profile, err)
	}

	// Try to get credentials to validate they exist
	credentials, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return fmt.Errorf("failed to retrieve AWS credentials for profile '%s': %w", awsConfig.Profile, err)
	}

	if credentials.AccessKeyID == "" {
		return fmt.Errorf("AWS access key ID is empty for profile '%s'", awsConfig.Profile)
	}

	log.Debug("AWS credentials validation successful", "access_key_id", maskAccessKey(credentials.AccessKeyID), "profile", awsConfig.Profile)
	return nil
}

// maskAccessKey masks most characters in an access key for logging
func maskAccessKey(accessKey string) string {
	if len(accessKey) <= 4 {
		return "****"
	}
	return accessKey[:4] + strings.Repeat("*", len(accessKey)-4)
}

// getNextPortFromConfig finds the next available port by examining existing configurations
func getNextPortFromConfig(configs []ProxyConfig) int {
	if len(configs) == 0 {
		return 3001 // Start from 3001 as a reasonable default for first RDS endpoint
	}

	// Find the highest used port
	maxPort := 0
	for _, config := range configs {
		if config.LocalPort > maxPort {
			maxPort = config.LocalPort
		}
	}

	// Return next port, but ensure it's at least 3000
	nextPort := maxPort + 1
	if nextPort < 3000 {
		nextPort = 3001
	}

	return nextPort
}

// GetStartingPortForAWSConfigs determines the starting port for new AWS configurations
func GetStartingPortForAWSConfigs(existingConfigs []ProxyConfig) int {
	return getNextPortFromConfig(existingConfigs)
}

// FilterRDSEndpointsByEngine filters RDS endpoints by engine type
func FilterRDSEndpointsByEngine(endpoints []RDSEndpoint, engines []string) []RDSEndpoint {
	if len(engines) == 0 {
		return endpoints
	}

	engineSet := make(map[string]bool)
	for _, engine := range engines {
		engineSet[strings.ToLower(engine)] = true
	}

	var filtered []RDSEndpoint
	for _, endpoint := range endpoints {
		if engineSet[strings.ToLower(endpoint.Engine)] {
			filtered = append(filtered, endpoint)
		}
	}

	log.Debug("Filtered RDS endpoints by engine",
		"original_count", len(endpoints),
		"filtered_count", len(filtered),
		"engines", engines)

	return filtered
}

// FilterRDSEndpointsByName filters RDS endpoints by name patterns
func FilterRDSEndpointsByName(endpoints []RDSEndpoint, names []string) []RDSEndpoint {
	if len(names) == 0 {
		return endpoints
	}

	var filtered []RDSEndpoint
	for _, endpoint := range endpoints {
		for _, name := range names {
			// Skip empty names
			trimmedName := strings.TrimSpace(name)
			if trimmedName == "" {
				continue
			}
			// Case-insensitive substring matching
			if strings.Contains(strings.ToLower(endpoint.Identifier), strings.ToLower(trimmedName)) {
				filtered = append(filtered, endpoint)
				break // Found a match, no need to check other names for this endpoint
			}
		}
	}

	log.Debug("Filtered RDS endpoints by name",
		"original_count", len(endpoints),
		"filtered_count", len(filtered),
		"names", names)

	return filtered
}

// FilterRDSEndpointsByStatus filters RDS endpoints by status
func FilterRDSEndpointsByStatus(endpoints []RDSEndpoint, statuses []string) []RDSEndpoint {
	if len(statuses) == 0 {
		// Default to available statuses
		statuses = []string{"available", "running"}
	}

	statusSet := make(map[string]bool)
	for _, status := range statuses {
		statusSet[strings.ToLower(status)] = true
	}

	var filtered []RDSEndpoint
	for _, endpoint := range endpoints {
		if statusSet[strings.ToLower(endpoint.Status)] {
			filtered = append(filtered, endpoint)
		}
	}

	log.Debug("Filtered RDS endpoints by status",
		"original_count", len(endpoints),
		"filtered_count", len(filtered),
		"statuses", statuses)

	return filtered
}
