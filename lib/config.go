package lib

// ProxyConfig represents a single proxy configuration
type ProxyConfig struct {
	Name              string `json:"name" mapstructure:"name"`
	KubernetesCluster string `json:"kubernetes_cluster" mapstructure:"kubernetes_cluster"`
	RemoteHost        string `json:"remote_host" mapstructure:"remote_host"`
	LocalPort         int    `json:"local_port" mapstructure:"local_port"`
	RemotePort        int    `json:"remote_port" mapstructure:"remote_port"`
}

// AppConfig represents the main application configuration
type AppConfig struct {
	ProxyConfigs []ProxyConfig `json:"proxy_configs" mapstructure:"proxy_configs"`
}
