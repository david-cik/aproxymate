# Aproxymate

Aproxymate is a command-line tool that simplifies creating and managing socat proxy pods in Kubernetes clusters through a web-based GUI and configuration management system, particularly useful for accessing remote services through Kubernetes pods.

## Overview

Aproxymate creates pods in your Kubernetes cluster that run `alpine/socat` to proxy TCP connections from the pod to remote hosts. This allows you to:

- Create temporary proxies to access remote services from within the cluster
- Easily tunnel connections to services that are only accessible from within your Kubernetes environment
- Set up short-lived connections without modifying service meshes or ingress configurations
- Manage multiple proxy configurations through a user-friendly web interface
- Save and reuse proxy configurations via YAML config files

## Installation

```bash
# Build from source
git clone https://github.com/yourusername/aproxymate.git
cd aproxymate
go build -o aproxymate .
```

## Usage

### Start the Web GUI

```bash
aproxymate gui
```

This starts a web-based interface at `http://localhost:8080` where you can:

- Add and configure multiple proxy connections
- Start and stop proxy pods with a single click
- Monitor connection status
- Save configurations for future use

You can specify a custom port:

```bash
aproxymate gui --port 9090
```

### Configuration Management

#### Create a sample configuration file

```bash
aproxymate config init
```

This creates a sample `aproxymate.yaml` file in your home directory with example proxy configurations.

#### Show configuration status

```bash
aproxymate config show
```

Displays the current configuration file location and status.

#### List all proxy configurations

```bash
aproxymate config list
```

Shows all proxy configurations defined in your config file.

### Using a custom configuration file

```bash
aproxymate gui --config /path/to/your/config.yaml
```

Load the GUI with a specific configuration file.

## Examples

### Getting started with a sample configuration

```bash
# Create a sample configuration file
aproxymate config init

# Start the GUI with the sample configurations
aproxymate gui
```

Then open `http://localhost:8080` in your browser to manage your proxy connections.

### Access a database only accessible from within the cluster

1. Create or edit your `aproxymate.yaml` configuration file:

```yaml
proxy_configs:
  - name: "Internal Database"
    kubernetes_cluster: "prod-cluster"
    remote_host: "internal-db.namespace.svc.cluster.local"
    remote_port: 5432
    local_port: 5432
```

2. Start the GUI:

```bash
aproxymate gui --config aproxymate.yaml
```

3. In the web interface, start the "Internal Database" proxy

4. Connect to the database:

```bash
psql -h localhost -p 5432 -U myuser mydatabase
```

### Managing multiple environments

Create a configuration file with multiple proxy settings:

```yaml
proxy_configs:
  - name: "PostgreSQL Production"
    kubernetes_cluster: "prod-cluster"
    remote_host: "postgres-service"
    remote_port: 5432
    local_port: 5432
  - name: "Redis Staging"
    kubernetes_cluster: "staging-cluster"
    remote_host: "redis-service"
    remote_port: 6379
    local_port: 6379
  - name: "MySQL Development"
    kubernetes_cluster: "dev-cluster"
    remote_host: "mysql-service"
    remote_port: 3306
    local_port: 3306
```

Then use the GUI to start and stop individual proxy connections as needed.

## Configuration

Aproxymate uses YAML configuration files to manage proxy settings and your kubeconfig file to connect to Kubernetes clusters.

### Configuration File Format

The configuration file (`aproxymate.yaml`) uses the following format:

```yaml
proxy_configs:
  - name: "Display Name"
    kubernetes_cluster: "cluster-context-name"
    remote_host: "target-hostname-or-service"
    remote_port: 5432
    local_port: 5432
```

### Configuration File Locations

Aproxymate looks for configuration files in the following order:

1. Path specified with `--config` flag
2. `$HOME/aproxymate.yaml`
3. `$HOME/.aproxymate.yaml`
4. `./aproxymate.yaml`
5. `./.aproxymate.yaml`

### Kubernetes Configuration

Aproxymate uses your kubeconfig file to connect to Kubernetes clusters. You can specify:

- `--config`: Path to the aproxymate configuration file
- The `kubernetes_cluster` field in your config should match a context name in your kubeconfig file

### Available Commands

```bash
aproxymate                    # Show configuration status and available options
aproxymate gui               # Start the web GUI (default port 8080)
aproxymate gui --port 9090   # Start GUI on custom port
aproxymate config init       # Create sample configuration file
aproxymate config show       # Show configuration file status
aproxymate config list       # List all proxy configurations
aproxymate --help           # Show help
```

## License

[MIT License](LICENSE)
