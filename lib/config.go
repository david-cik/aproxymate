package lib

import (
	"fmt"

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
		if proxy.KubernetesCluster == "" {
			return fmt.Errorf("proxy config #%d (%s) is missing 'kubernetes_cluster' field", i+1, proxy.Name)
		}
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
