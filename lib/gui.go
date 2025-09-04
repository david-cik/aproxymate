package lib

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/viper"

	log "aproxymate/lib/logger"
)

//go:embed templates/index.html
var indexHTML string

// ProxyRow represents a single proxy configuration row
type ProxyRow struct {
	ID                string    `json:"id"`
	KubernetesCluster string    `json:"cluster"`
	RemoteHost        string    `json:"host"`
	LocalPort         int       `json:"localPort"`
	RemotePort        int       `json:"remotePort"`
	Connected         bool      `json:"connected"`
	Process           *exec.Cmd `json:"-"`
	SocatPodName      string    `json:"-"` // Name of the socat pod
	SocatNamespace    string    `json:"-"` // Namespace for the socat pod
	IntentionalStop   bool      `json:"-"` // Flag to track if stop was intentional
}

// GuiData holds the data for the HTML template
type GuiData struct {
	ProxyRows []*ProxyRow
	NextID    int
}

// GUI manages the web interface and proxy connections
type GUI struct {
	mu               sync.RWMutex
	rows             map[string]*ProxyRow
	nextID           int
	server           *http.Server
	configFileLoaded bool // Track if a config file was actually loaded
}

// NewGUI creates a new GUI instance
func NewGUI() *GUI {
	gui := &GUI{
		rows:   make(map[string]*ProxyRow),
		nextID: 1,
	}

	// Create one default empty row
	defaultRow := &ProxyRow{
		ID:                "1",
		KubernetesCluster: "",
		RemoteHost:        "",
		LocalPort:         0,
		RemotePort:        0,
		Connected:         false,
	}
	gui.rows["1"] = defaultRow
	gui.nextID = 2

	return gui
}

// LoadConfigFromViper loads proxy configurations from Viper config
func (g *GUI) LoadConfigFromViper() (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	var config AppConfig
	if err := viper.Unmarshal(&config); err != nil {
		return 0, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Check if we actually loaded proxy configs (indicating a real config file was read)
	configFileUsed := viper.ConfigFileUsed()
	g.configFileLoaded = len(config.ProxyConfigs) > 0 && configFileUsed != ""

	// Log configuration validation information
	if configFileUsed != "" {
		opCtx, _ := log.StartOperation(context.Background(), "gui", "load_config")
		defer opCtx.Complete("load_config", nil)

		opCtx.Debug("GUI loading configuration from file", "file", configFileUsed, "num_configs", len(config.ProxyConfigs))
		log.LogConfigLoad(configFileUsed, len(config.ProxyConfigs))

		// Simple validation - check for missing required fields
		validationErrors := 0
		for i, proxy := range config.ProxyConfigs {
			if proxy.Name == "" {
				opCtx.Warn("Configuration validation warning", "issue", "missing name", "config_index", i+1)
				validationErrors++
			}
			if proxy.KubernetesCluster == "" {
				opCtx.Warn("Configuration validation warning", "issue", "missing kubernetes_cluster", "config_index", i+1, "name", proxy.Name)
				validationErrors++
			}
			if proxy.RemoteHost == "" {
				opCtx.Warn("Configuration validation warning", "issue", "missing remote_host", "config_index", i+1, "name", proxy.Name)
				validationErrors++
			}
			if proxy.LocalPort == 0 {
				opCtx.Warn("Configuration validation warning", "issue", "invalid local_port", "config_index", i+1, "name", proxy.Name, "port", proxy.LocalPort)
				validationErrors++
			}
			if proxy.RemotePort == 0 {
				opCtx.Warn("Configuration validation warning", "issue", "invalid remote_port", "config_index", i+1, "name", proxy.Name, "port", proxy.RemotePort)
				validationErrors++
			}
		}

		if validationErrors > 0 {
			opCtx.Warn("Configuration validation completed with warnings", "total_errors", validationErrors)
		} else {
			opCtx.Debug("Configuration validation completed successfully")
		}

		// Check for missing clusters and prompt if needed
		if HasConfigsWithMissingClusters(config.ProxyConfigs) {
			missingConfigs := FindConfigsWithMissingClusters(config.ProxyConfigs)
			opCtx.Debug("Found configurations with missing Kubernetes clusters", "count", len(missingConfigs))

			selectedCluster, err := SelectKubernetesClusterTUI("")
			if err != nil {
				return 0, fmt.Errorf("failed to select Kubernetes cluster: %w", err)
			}

			// Update all configs with missing clusters
			config.ProxyConfigs = UpdateConfigsWithCluster(config.ProxyConfigs, selectedCluster)
			log.Debug("Updated configurations with selected cluster", "cluster", selectedCluster, "updated_count", len(missingConfigs))

			// Save the updated configuration back to the file
			if configFileUsed != "" {
				viper.Set("proxy_configs", config.ProxyConfigs)
				if err := viper.WriteConfig(); err != nil {
					outputCtx := NewSimpleOutputContext()
					outputCtx.Warn("Failed to save updated configuration with cluster information", "Warning: Could not save updated configuration: %v\n", err)
				} else {
					outputCtx := NewSimpleOutputContext()
					outputCtx.Success("Saved updated configuration with cluster information", "✅ Updated configuration saved with cluster '%s'\n", selectedCluster)
				}
			}
		}
	} else {
		log.Debug("No configuration file loaded - using default empty configuration")
	}

	// If we have actual proxy configs, clear the default empty row and load the configs
	if len(config.ProxyConfigs) > 0 {
		// Clear existing rows (including the default empty row)
		g.rows = make(map[string]*ProxyRow)
		g.nextID = 1

		// Load proxy configurations
		for i, proxyConfig := range config.ProxyConfigs {
			id := strconv.Itoa(i + 1)
			row := &ProxyRow{
				ID:                id,
				KubernetesCluster: proxyConfig.KubernetesCluster,
				RemoteHost:        proxyConfig.RemoteHost,
				LocalPort:         proxyConfig.LocalPort,
				RemotePort:        proxyConfig.RemotePort,
				Connected:         false,
			}
			g.rows[id] = row

			// Update nextID to be after the loaded configs
			if nextID, err := strconv.Atoi(id); err == nil && nextID >= g.nextID {
				g.nextID = nextID + 1
			}
		}
	}

	return len(config.ProxyConfigs), nil
}

// Start starts the GUI web server
func (g *GUI) Start(port int, serverReady chan<- bool) error {
	// Load configuration from Viper
	if numrows, err := g.LoadConfigFromViper(); err != nil {
		log.Warn("Failed to load configuration", "error", err)
	} else if numrows > 0 {
		log.Debug("Loaded proxy configurations for GUI", "count", numrows)
	} else {
		log.Debug("Starting GUI with empty configuration")
	}

	// Clean up any orphaned aproxymate pods from previous sessions
	log.Debug("Starting orphaned pod cleanup")
	contexts, err := GetKubernetesContexts("")
	if err != nil {
		log.Warn("Could not get Kubernetes contexts for cleanup", "error", err)
	} else {
		for _, contextName := range contexts {
			kubeClient, err := GetKubernetesClient(KubeConfig{Context: contextName})
			if err != nil {
				log.Warn("Could not create Kubernetes client for cleanup", "context", contextName, "error", err)
				continue
			}

			if err := CleanupOrphanedAproxymatePodsForUser(kubeClient, "default"); err != nil {
				log.Warn("Failed to cleanup orphaned pods", "context", contextName, "error", err)
			}
		}
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Info("Received shutdown signal, cleaning up", "signal", sig.String())
		g.cleanupAllPods()
		os.Exit(0)
	}()

	mux := http.NewServeMux()

	// Serve the main page
	mux.HandleFunc("/", g.handleIndex)

	// API endpoints
	mux.HandleFunc("/api/proxy", g.handleProxy)
	mux.HandleFunc("/api/proxy/", g.handleProxyWithID)
	mux.HandleFunc("/api/connect", g.handleConnect)
	mux.HandleFunc("/api/disconnect/", g.handleDisconnect)
	mux.HandleFunc("/api/contexts", g.handleContexts)
	mux.HandleFunc("/api/config/save", g.handleSaveConfig)
	mux.HandleFunc("/api/config/location", g.handleConfigLocation)
	mux.HandleFunc("/api/status", g.handleStatus)

	g.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	outputCtx := NewSimpleOutputContext()
	outputCtx.Info("GUI server starting", "Aproxymate GUI starting on http://localhost:%d\n", port)

	// Start the server in a goroutine
	go func() {
		if err := g.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("GUI server failed to start", "error", err)
		}
	}()

	// Wait for server to be ready by trying to connect to it
	for i := 0; i < 30; i++ { // Try for up to 3 seconds
		if g.isServerReady(port) {
			if serverReady != nil {
				close(serverReady)
			}
			log.Debug("GUI server is ready and accepting connections", "port", port)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Block indefinitely to keep the server running
	select {}
}

// isServerReady checks if the GUI server is ready to accept connections
func (g *GUI) isServerReady(port int) bool {
	client := &http.Client{
		Timeout: 50 * time.Millisecond,
	}

	url := fmt.Sprintf("http://localhost:%d/api/status", port)
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// handleIndex serves the main HTML page
func (g *GUI) handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.New("index").Parse(indexHTML)
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	g.mu.RLock()
	rows := make([]*ProxyRow, 0, len(g.rows))
	for _, row := range g.rows {
		rows = append(rows, row)
	}
	nextID := g.nextID
	g.mu.RUnlock()

	// Sort rows by ID to preserve the order from the config file
	sort.Slice(rows, func(i, j int) bool {
		idI, errI := strconv.Atoi(rows[i].ID)
		idJ, errJ := strconv.Atoi(rows[j].ID)

		// If both IDs are valid numbers, sort numerically
		if errI == nil && errJ == nil {
			return idI < idJ
		}

		// Fall back to string comparison for non-numeric IDs
		return rows[i].ID < rows[j].ID
	})

	data := GuiData{
		ProxyRows: rows,
		NextID:    nextID,
	}

	w.Header().Set("Content-Type", "text/html")
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Template execution error: "+err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleProxy handles POST requests to create/update proxy configurations
func (g *GUI) handleProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID                string `json:"id"`
		KubernetesCluster string `json:"cluster"`
		RemoteHost        string `json:"host"`
		LocalPort         int    `json:"localPort"`
		RemotePort        int    `json:"remotePort"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	row := &ProxyRow{
		ID:                req.ID,
		KubernetesCluster: req.KubernetesCluster,
		RemoteHost:        req.RemoteHost,
		LocalPort:         req.LocalPort,
		RemotePort:        req.RemotePort,
		Connected:         false,
	}

	g.rows[req.ID] = row

	// Update nextID if necessary
	if id, err := strconv.Atoi(req.ID); err == nil && id >= g.nextID {
		g.nextID = id + 1
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// handleProxyWithID handles DELETE requests for specific proxy configurations
func (g *GUI) handleProxyWithID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/api/proxy/"):]

	g.mu.Lock()
	defer g.mu.Unlock()

	if row, exists := g.rows[id]; exists {
		// Stop the proxy if it's running
		if row.Connected && row.Process != nil {
			row.Process.Process.Kill()
		}
		delete(g.rows, id)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// handleConnect handles POST requests to start a proxy connection
func (g *GUI) handleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID                string `json:"id"`
		KubernetesCluster string `json:"cluster"`
		RemoteHost        string `json:"host"`
		LocalPort         int    `json:"localPort"`
		RemotePort        int    `json:"remotePort"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	log.Debug("Processing proxy connection request",
		"cluster", req.KubernetesCluster,
		"host", req.RemoteHost,
		"local_port", req.LocalPort,
		"remote_port", req.RemotePort)

	g.mu.Lock()
	defer g.mu.Unlock()

	row, exists := g.rows[req.ID]
	if !exists {
		row = &ProxyRow{
			ID:                req.ID,
			KubernetesCluster: req.KubernetesCluster,
			RemoteHost:        req.RemoteHost,
			LocalPort:         req.LocalPort,
			RemotePort:        req.RemotePort,
		}
		g.rows[req.ID] = row
	}

	if row.Connected {
		http.Error(w, "Proxy already connected", http.StatusBadRequest)
		return
	}

	// Create Kubernetes client
	kubeClient, err := GetKubernetesClient(KubeConfig{
		Context: req.KubernetesCluster,
	})
	if err != nil {
		log.Error("Failed to create Kubernetes client", "cluster", req.KubernetesCluster, "error", err)
		http.Error(w, fmt.Sprintf("Cannot connect to Kubernetes cluster '%s'. Please check if the cluster is accessible and your kubeconfig is valid. Error: %v", req.KubernetesCluster, err), http.StatusInternalServerError)
		return
	}

	// Generate unique pod name with username
	username := getSafeUsername()
	podName := fmt.Sprintf("aproxymate-%s-%s-%d", username, req.ID, time.Now().Unix())
	namespace := "default" // You might want to make this configurable

	// Create socat proxy pod configuration
	socatConfig := SocatProxyConfig{
		PodName:    podName,
		Namespace:  namespace,
		ListenPort: req.RemotePort, // The port the socat pod will listen on
		RemoteHost: req.RemoteHost,
		RemotePort: req.RemotePort,
	}

	log.Info("Creating socat proxy pod",
		"pod", podName,
		"namespace", namespace,
		"target_host", req.RemoteHost,
		"target_port", req.RemotePort)

	// Create the socat proxy pod
	pod, err := CreateSocatProxyPod(kubeClient, socatConfig)
	if err != nil {
		log.Error("Failed to create socat proxy pod", "pod", podName, "cluster", req.KubernetesCluster, "error", err)
		http.Error(w, fmt.Sprintf("Failed to create proxy pod in Kubernetes cluster '%s'. This could be due to insufficient permissions, network issues, or cluster configuration problems. Error: %v", req.KubernetesCluster, err), http.StatusInternalServerError)
		return
	}

	log.Info("Socat pod created, waiting for running state", "pod", pod.Name, "namespace", namespace)

	// Wait for the pod to be running
	if err := WaitForPodRunning(kubeClient, namespace, podName, 30*time.Second); err != nil {
		log.Error("Pod failed to start", "pod", podName, "namespace", namespace, "error", err)
		// Clean up the pod
		DeleteSocatProxyPod(kubeClient, namespace, podName)
		http.Error(w, fmt.Sprintf("Proxy pod failed to start within 30 seconds. This could be due to resource constraints, image pull issues, or networking problems in cluster '%s'. Error: %v", req.KubernetesCluster, err), http.StatusInternalServerError)
		return
	}

	log.Info("Socat pod is running, starting kubectl port-forward", "pod", podName, "local_port", req.LocalPort, "remote_port", req.RemotePort)

	// Now start kubectl port-forward to the socat pod
	cmd := exec.Command("kubectl",
		"port-forward",
		fmt.Sprintf("pod/%s", podName),
		fmt.Sprintf("%d:%d", req.LocalPort, req.RemotePort),
		"--context", req.KubernetesCluster,
		"--namespace", namespace,
	)

	// Capture stderr to see kubectl errors
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	log.Debug("Starting kubectl port-forward command", "command", cmd.String(), "cluster", req.KubernetesCluster)

	if err := cmd.Start(); err != nil {
		log.Error("Failed to start kubectl port-forward", "command", cmd.String(), "error", err)
		// Clean up the pod
		DeleteSocatProxyPod(kubeClient, namespace, podName)

		// Provide more specific error messages based on the error type
		errorMsg := fmt.Sprintf("Failed to start port forwarding to local port %d", req.LocalPort)

		// Check for common port binding issues
		if strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "bind: permission denied") {
			if req.LocalPort <= 1023 {
				errorMsg = fmt.Sprintf("Permission denied: Port %d is a privileged port (1-1023) that requires administrator privileges. Please try using a port above 1023 or run with elevated permissions", req.LocalPort)
			} else {
				errorMsg = fmt.Sprintf("Permission denied binding to port %d. Please check your system permissions", req.LocalPort)
			}
		} else if strings.Contains(err.Error(), "address already in use") || strings.Contains(err.Error(), "bind: address already in use") {
			errorMsg = fmt.Sprintf("Port %d is already in use by another service. Please choose a different local port or stop the service using port %d", req.LocalPort, req.LocalPort)
		} else if strings.Contains(err.Error(), "kubectl") {
			errorMsg = fmt.Sprintf("kubectl command failed. Please ensure kubectl is installed and properly configured. Error: %v", err)
		}

		http.Error(w, errorMsg, http.StatusInternalServerError)
		return
	}

	// Give the command a moment to start properly
	time.Sleep(500 * time.Millisecond)

	// Check if the process is still running
	if cmd.Process == nil {
		log.Error("kubectl port-forward process failed to start properly", "cluster", req.KubernetesCluster)
		DeleteSocatProxyPod(kubeClient, namespace, podName)
		http.Error(w, fmt.Sprintf("Port forwarding failed to initialize properly. This might indicate a problem with kubectl or the Kubernetes cluster connection for '%s'.", req.KubernetesCluster), http.StatusInternalServerError)
		return
	}

	// Check if the process has already exited
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		exitCode := cmd.ProcessState.ExitCode()
		log.Error("kubectl port-forward process exited immediately", "exit_code", exitCode, "cluster", req.KubernetesCluster)
		DeleteSocatProxyPod(kubeClient, namespace, podName)

		// Provide specific error messages based on exit code
		var errorMsg string
		switch exitCode {
		case 1:
			if req.LocalPort <= 1023 {
				errorMsg = fmt.Sprintf("Port forwarding failed: Port %d is a privileged port (1-1023) that requires administrator privileges. Please try using a port above 1023 (e.g., 8080, 9000) or run with elevated permissions", req.LocalPort)
			} else {
				errorMsg = fmt.Sprintf("Port forwarding failed: Port %d is likely already in use by another service. Please try a different local port or stop the service using port %d", req.LocalPort, req.LocalPort)
			}
		case 2:
			errorMsg = fmt.Sprintf("Port forwarding failed due to incorrect usage or invalid arguments. Please check if cluster '%s' is accessible and the configuration is correct", req.KubernetesCluster)
		default:
			errorMsg = fmt.Sprintf("Port forwarding failed immediately (exit code %d). This usually means local port %d is already in use, requires elevated permissions, or there was a network/authentication issue with cluster '%s'. Please try a different local port or check your cluster connection", exitCode, req.LocalPort, req.KubernetesCluster)
		}

		http.Error(w, errorMsg, http.StatusInternalServerError)
		return
	}

	// Update row with connection info
	row.Process = cmd
	row.Connected = true
	row.SocatPodName = podName
	row.SocatNamespace = namespace

	log.Info("Successfully started proxy connection",
		"cluster", req.KubernetesCluster,
		"host", req.RemoteHost,
		"local_port", req.LocalPort,
		"remote_port", req.RemotePort,
		"pod", podName,
		"pid", cmd.Process.Pid)

	// Monitor the process in a goroutine
	go func() {
		err := cmd.Wait()
		g.mu.Lock()
		if r, exists := g.rows[req.ID]; exists {
			r.Connected = false
			r.Process = nil

			// Clean up the socat pod
			if r.SocatPodName != "" {
				log.Debug("Cleaning up socat pod after connection ended", "pod", r.SocatPodName, "namespace", r.SocatNamespace)
				if kubeClient, err := GetKubernetesClient(KubeConfig{Context: r.KubernetesCluster}); err == nil {
					DeleteSocatProxyPod(kubeClient, r.SocatNamespace, r.SocatPodName)
				}
				r.SocatPodName = ""
				r.SocatNamespace = ""
			}

			if err != nil {
				// Check if this was an intentional stop
				if r.IntentionalStop {
					log.Info("Port-forward stopped intentionally",
						"cluster", r.KubernetesCluster,
						"host", r.RemoteHost,
						"local_port", r.LocalPort,
						"remote_port", r.RemotePort)
				} else {
					log.Error("Port-forward exited with error",
						"cluster", r.KubernetesCluster,
						"host", r.RemoteHost,
						"local_port", r.LocalPort,
						"remote_port", r.RemotePort,
						"error", err)
				}
			} else {
				log.Info("Port-forward exited normally",
					"cluster", r.KubernetesCluster,
					"host", r.RemoteHost,
					"local_port", r.LocalPort,
					"remote_port", r.RemotePort)
			}

			// Reset the intentional stop flag
			r.IntentionalStop = false
		}
		g.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// handleDisconnect handles POST requests to stop a proxy connection
func (g *GUI) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/api/disconnect/"):]

	g.mu.Lock()
	defer g.mu.Unlock()

	row, exists := g.rows[id]
	if !exists {
		availableIDs := func() []string {
			var ids []string
			for k := range g.rows {
				ids = append(ids, k)
			}
			return ids
		}()
		log.Warn("Disconnect request for non-existent row", "requested_id", id, "available_ids", availableIDs)
		http.Error(w, "Proxy not found", http.StatusBadRequest)
		return
	}

	log.Info("Disconnect request received",
		"id", id,
		"cluster", row.KubernetesCluster,
		"host", row.RemoteHost,
		"local_port", row.LocalPort,
		"remote_port", row.RemotePort)

	if !row.Connected {
		log.Warn("Disconnect request for already disconnected proxy", "id", id)
		http.Error(w, "Proxy not connected", http.StatusBadRequest)
		return
	}

	// Kill the kubectl port-forward process
	if row.Process != nil {
		row.IntentionalStop = true // Mark as intentional stop
		if err := row.Process.Process.Kill(); err != nil {
			log.Error("Error killing kubectl process",
				"cluster", row.KubernetesCluster,
				"host", row.RemoteHost,
				"local_port", row.LocalPort,
				"remote_port", row.RemotePort,
				"error", err)
		}
		row.Process = nil
	}

	// Clean up the socat pod
	if row.SocatPodName != "" {
		log.Debug("Cleaning up socat pod", "pod", row.SocatPodName, "namespace", row.SocatNamespace)
		kubeClient, err := GetKubernetesClient(KubeConfig{Context: row.KubernetesCluster})
		if err != nil {
			log.Error("Failed to create Kubernetes client for cleanup", "cluster", row.KubernetesCluster, "error", err)
		} else {
			if err := DeleteSocatProxyPod(kubeClient, row.SocatNamespace, row.SocatPodName); err != nil {
				log.Error("Error deleting socat pod", "pod", row.SocatPodName, "namespace", row.SocatNamespace, "error", err)
			} else {
				log.Debug("Successfully deleted socat pod", "pod", row.SocatPodName, "namespace", row.SocatNamespace)
			}
		}
		row.SocatPodName = ""
		row.SocatNamespace = ""
	}

	row.Connected = false
	log.Info("Successfully disconnected proxy",
		"cluster", row.KubernetesCluster,
		"host", row.RemoteHost,
		"local_port", row.LocalPort,
		"remote_port", row.RemotePort)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// handleContexts handles GET requests to fetch available Kubernetes contexts
func (g *GUI) handleContexts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	contexts, err := GetKubernetesContexts("")
	if err != nil {
		http.Error(w, "Failed to get contexts: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]string{"contexts": contexts})
}

// handleSaveConfig handles saving the current configuration to file
func (g *GUI) handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if we have ordered rows data from the frontend
	var orderedRowsRequest struct {
		OrderedRows []struct {
			ID         string `json:"id"`
			Order      int    `json:"order"`
			Cluster    string `json:"cluster"`
			Host       string `json:"host"`
			LocalPort  int    `json:"localPort"`
			RemotePort int    `json:"remotePort"`
		} `json:"orderedRows"`
	}

	// Try to decode the request body
	if err := json.NewDecoder(r.Body).Decode(&orderedRowsRequest); err != nil {
		// If JSON decode fails, fall back to using current rows in arbitrary order
		log.Debug("No ordered rows data provided, using current rows", "error", err)
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	var configs []ProxyConfig

	if len(orderedRowsRequest.OrderedRows) > 0 {
		// Use ordered rows from frontend
		log.Debug("Saving configuration with preserved order", "ordered_rows", len(orderedRowsRequest.OrderedRows))

		// Sort by order field to ensure correct sequence
		orderedRows := orderedRowsRequest.OrderedRows
		for i := 0; i < len(orderedRows); i++ {
			for j := i + 1; j < len(orderedRows); j++ {
				if orderedRows[i].Order > orderedRows[j].Order {
					orderedRows[i], orderedRows[j] = orderedRows[j], orderedRows[i]
				}
			}
		}

		// Convert ordered rows to config format
		for _, orderedRow := range orderedRows {
			// Skip empty configurations
			if orderedRow.Cluster == "" && orderedRow.Host == "" && orderedRow.LocalPort == 0 && orderedRow.RemotePort == 0 {
				continue
			}

			config := ProxyConfig{
				Name:              fmt.Sprintf("%s:%d", orderedRow.Host, orderedRow.LocalPort),
				KubernetesCluster: orderedRow.Cluster,
				RemoteHost:        orderedRow.Host,
				LocalPort:         orderedRow.LocalPort,
				RemotePort:        orderedRow.RemotePort,
			}
			configs = append(configs, config)
		}
	} else {
		// Fall back to current rows (arbitrary order)
		log.Debug("No order specified, saving rows in arbitrary order", "row_count", len(g.rows))
		for _, row := range g.rows {
			// Skip empty configurations
			if row.KubernetesCluster == "" && row.RemoteHost == "" && row.LocalPort == 0 && row.RemotePort == 0 {
				continue
			}

			config := ProxyConfig{
				Name:              fmt.Sprintf("%s:%d", row.RemoteHost, row.LocalPort),
				KubernetesCluster: row.KubernetesCluster,
				RemoteHost:        row.RemoteHost,
				LocalPort:         row.LocalPort,
				RemotePort:        row.RemotePort,
			}
			configs = append(configs, config)
		}
	}

	// Save to Viper and write to file
	viper.Set("proxy_configs", configs)

	var savedConfigFile string

	if !g.configFileLoaded {
		// No config file was initially loaded, use default location
		configFile := GetLocalConfigPath()
		// Convert to absolute path for display and consistency
		absConfigFile := GetAbsolutePathForDisplay(configFile)
		log.Info("No config file was loaded on startup, saving to default location", "file", absConfigFile)
		savedConfigFile = absConfigFile

		// Force write using WriteConfigAs to ensure we get the correct filename
		err := viper.WriteConfigAs(configFile)
		if err != nil {
			log.Error("Error saving configuration", "file", configFile, "error", err)
			http.Error(w, fmt.Sprintf("Failed to save configuration: %v", err), http.StatusInternalServerError)
			return
		}

		// Now that we've saved a config file, mark it as loaded for future saves
		// and set viper to use this file
		viper.SetConfigFile(configFile)
		g.configFileLoaded = true
	} else {
		// Config file was loaded, try to write to the same location
		configFile := viper.ConfigFileUsed()
		err := viper.WriteConfig()
		if err != nil {
			log.Error("Error writing to existing config file", "file", configFile, "error", err)
			http.Error(w, fmt.Sprintf("Failed to save configuration: %v", err), http.StatusInternalServerError)
			return
		}
		// Convert to absolute path for display consistency
		savedConfigFile = GetAbsolutePathForDisplay(configFile)
	}

	log.Info("Configuration saved successfully", "proxy_configs", len(configs), "file", savedConfigFile, "order_preserved", len(orderedRowsRequest.OrderedRows) > 0)

	w.Header().Set("Content-Type", "application/json")
	response := map[string]string{"status": "success", "message": "Configuration saved successfully"}
	json.NewEncoder(w).Encode(response)
}

// handleConfigLocation handles GET requests to retrieve the current config file location
func (g *GUI) handleConfigLocation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	location := g.GetConfigSaveLocation()
	nextSaveLocation := GetLocalConfigPath()

	if g.configFileLoaded {
		configFile := viper.ConfigFileUsed()
		if configFile != "" {
			nextSaveLocation = configFile
		}
	}

	// Convert nextSaveLocation to absolute path for consistent display
	absNextSaveLocation := GetAbsolutePathForDisplay(nextSaveLocation)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"location":         location,
		"nextSaveLocation": absNextSaveLocation,
		"loaded":           fmt.Sprintf("%t", g.configFileLoaded),
	})
}

// handleStatus handles GET requests to check the status of all proxies
func (g *GUI) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	g.mu.RLock()
	defer g.mu.RUnlock()

	// Check actual process status and update accordingly
	for id, row := range g.rows {
		if row.Process != nil {
			// Check if process is still running
			if row.Process.ProcessState != nil && row.Process.ProcessState.Exited() {
				log.Debug("Process has exited, updating status", "id", id, "exit_code", row.Process.ProcessState.ExitCode())
				row.Connected = false
				row.Process = nil
			}
		}
	}

	// Return current status
	status := make(map[string]bool)
	for id, row := range g.rows {
		status[id] = row.Connected
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": status,
	})
}

// cleanupAllPods cleans up all socat pods managed by this GUI instance
func (g *GUI) cleanupAllPods() {
	g.mu.RLock()
	defer g.mu.RUnlock()

	log.Info("Cleaning up all active socat pods")

	for _, row := range g.rows {
		if row.Connected && row.SocatPodName != "" {
			log.Debug("Cleaning up pod during shutdown",
				"cluster", row.KubernetesCluster,
				"host", row.RemoteHost,
				"local_port", row.LocalPort,
				"remote_port", row.RemotePort,
				"pod", row.SocatPodName)

			// Kill the kubectl process
			if row.Process != nil {
				row.Process.Process.Kill()
			}

			// Delete the pod
			kubeClient, err := GetKubernetesClient(KubeConfig{Context: row.KubernetesCluster})
			if err != nil {
				log.Warn("Failed to get Kubernetes client for pod cleanup",
					"cluster", row.KubernetesCluster,
					"error", err)
				continue
			}

			if err := DeleteSocatProxyPod(kubeClient, row.SocatNamespace, row.SocatPodName); err != nil {
				log.Warn("Failed to delete socat pod during cleanup",
					"cluster", row.KubernetesCluster,
					"namespace", row.SocatNamespace,
					"pod", row.SocatPodName,
					"error", err)
			} else {
				log.Debug("Successfully deleted socat pod",
					"cluster", row.KubernetesCluster,
					"namespace", row.SocatNamespace,
					"pod", row.SocatPodName)
			}
		}
	}
}

// GetConfigSaveLocation returns the location where the config will be saved
func (g *GUI) GetConfigSaveLocation() string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if !g.configFileLoaded {
		return "None"
	}

	configFile := viper.ConfigFileUsed()
	if configFile == "" {
		return "None"
	}

	// Convert to absolute path for consistent display
	return GetAbsolutePathForDisplay(configFile)
}

// getSafeUsername returns a Kubernetes-safe username
func getSafeUsername() string {
	currentUser, err := user.Current()
	if err != nil {
		return "unknown"
	}

	// Clean the username to be Kubernetes-safe (lowercase, no special chars except hyphens)
	username := strings.ToLower(currentUser.Username)
	// Replace any non-alphanumeric characters with hyphens
	var safeName strings.Builder
	for _, r := range username {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			safeName.WriteRune(r)
		} else {
			safeName.WriteRune('-')
		}
	}

	// Trim any leading/trailing hyphens and limit length
	result := strings.Trim(safeName.String(), "-")
	if len(result) > 20 {
		result = result[:20]
	}
	if result == "" {
		result = "user"
	}

	return result
}

// DisplayConfigurations prints all loaded proxy configurations
func (g *GUI) DisplayConfigurations() {
	g.mu.RLock()
	defer g.mu.RUnlock()

	log.Debug("Displaying proxy configurations", "count", len(g.rows))

	if len(g.rows) == 0 {
		fmt.Println("No proxy configurations loaded.")
		return
	}

	for _, row := range g.rows {
		fmt.Printf("ID: %s\n", row.ID)
		fmt.Printf("  Kubernetes Cluster: %s\n", row.KubernetesCluster)
		fmt.Printf("  Remote Host: %s\n", row.RemoteHost)
		fmt.Printf("  Local Port: %d\n", row.LocalPort)
		fmt.Printf("  Remote Port: %d\n", row.RemotePort)
		fmt.Printf("  Status: %s\n", func() string {
			if row.Connected {
				return "Connected"
			}
			return "Disconnected"
		}())
		fmt.Println()
	}
}

// Stop gracefully stops the GUI server
func (g *GUI) Stop() error {
	if g.server != nil {
		return g.server.Close()
	}
	return nil
}
