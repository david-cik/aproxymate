package lib

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/viper"
)

//go:embed templates/index.html
var indexHTML string

// ProxyRow represents a single proxy configuration row
type ProxyRow struct {
	ID                string `json:"id"`
	KubernetesCluster string `json:"cluster"`
	RemoteHost        string `json:"host"`
	LocalPort         int    `json:"localPort"`
	RemotePort        int    `json:"remotePort"`
	Connected         bool   `json:"connected"`
	Process           *exec.Cmd `json:"-"`
	SocatPodName      string `json:"-"`       // Name of the socat pod
	SocatNamespace    string `json:"-"`       // Namespace for the socat pod
	IntentionalStop   bool   `json:"-"`       // Flag to track if stop was intentional
}

// GuiData holds the data for the HTML template
type GuiData struct {
	ProxyRows []*ProxyRow
	NextID    int
}

// GUI manages the web interface and proxy connections
type GUI struct {
	mu              sync.RWMutex
	rows            map[string]*ProxyRow
	nextID          int
	server          *http.Server
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
func (g *GUI) Start(port int) error {
	// Load configuration from Viper
	if numrows, err := g.LoadConfigFromViper(); err != nil {
		log.Printf("Warning: Failed to load config: %v", err)
	} else if numrows > 0 {
		log.Printf("Loaded %d proxy configurations from config file", numrows)
	} else {
		log.Printf("No proxy configurations found in config file, starting with empty configuration")
	}

	// Clean up any orphaned aproxymate pods from previous sessions
	log.Println("Cleaning up any orphaned aproxymate pods...")
	contexts, err := GetKubernetesContexts("")
	if err != nil {
		log.Printf("Warning: Could not get Kubernetes contexts for cleanup: %v", err)
	} else {
		for _, contextName := range contexts {
			kubeClient, err := GetKubernetesClient(KubeConfig{Context: contextName})
			if err != nil {
				log.Printf("Warning: Could not create client for context %s: %v", contextName, err)
				continue
			}
			
			if err := CleanupOrphanedAproxymatePodsForUser(kubeClient, "default"); err != nil {
				log.Printf("Warning: Failed to cleanup pods in context %s: %v", contextName, err)
			}
		}
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, cleaning up...", sig)
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
	
	fmt.Printf("🚀 Aproxymate GUI starting on http://localhost:%d\n", port)
	return g.server.ListenAndServe()
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
	
	log.Printf("Connect request: cluster=%s, host=%s, ports=%d->%d", req.KubernetesCluster, req.RemoteHost, req.LocalPort, req.RemotePort)
	
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
		log.Printf("Failed to create Kubernetes client: %v", err)
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
	
	log.Printf("Creating socat proxy pod: %s in namespace %s for %s:%d", podName, namespace, req.RemoteHost, req.RemotePort)
	
	// Create the socat proxy pod
	pod, err := CreateSocatProxyPod(kubeClient, socatConfig)
	if err != nil {
		log.Printf("Failed to create socat proxy pod: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create proxy pod in Kubernetes cluster '%s'. This could be due to insufficient permissions, network issues, or cluster configuration problems. Error: %v", req.KubernetesCluster, err), http.StatusInternalServerError)
		return
	}
	
	log.Printf("Socat pod created: %s, waiting for it to be running...", pod.Name)
	
	// Wait for the pod to be running
	if err := WaitForPodRunning(kubeClient, namespace, podName, 30*time.Second); err != nil {
		log.Printf("Pod failed to start: %v", err)
		// Clean up the pod
		DeleteSocatProxyPod(kubeClient, namespace, podName)
		http.Error(w, fmt.Sprintf("Proxy pod failed to start within 30 seconds. This could be due to resource constraints, image pull issues, or networking problems in cluster '%s'. Error: %v", req.KubernetesCluster, err), http.StatusInternalServerError)
		return
	}
	
	log.Printf("Socat pod is running, starting kubectl port-forward to pod...")
	
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
	
	log.Printf("Starting kubectl port-forward command: %s", cmd.String())
	
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start kubectl port-forward: %v", err)
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
		log.Printf("kubectl port-forward process failed to start properly")
		DeleteSocatProxyPod(kubeClient, namespace, podName)
		http.Error(w, fmt.Sprintf("Port forwarding failed to initialize properly. This might indicate a problem with kubectl or the Kubernetes cluster connection for '%s'.", req.KubernetesCluster), http.StatusInternalServerError)
		return
	}
	
	// Check if the process has already exited
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		exitCode := cmd.ProcessState.ExitCode()
		log.Printf("kubectl port-forward process exited immediately with code: %d", exitCode)
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
	
	log.Printf("Successfully started proxy: cluster=%s, host=%s, ports=%d->%d, pod=%s, PID=%d", req.KubernetesCluster, req.RemoteHost, req.LocalPort, req.RemotePort, podName, cmd.Process.Pid)
	
	// Monitor the process in a goroutine
	go func() {
		err := cmd.Wait()
		g.mu.Lock()
		if r, exists := g.rows[req.ID]; exists {
			r.Connected = false
			r.Process = nil
			
			// Clean up the socat pod
			if r.SocatPodName != "" {
				log.Printf("Cleaning up socat pod: %s", r.SocatPodName)
				if kubeClient, err := GetKubernetesClient(KubeConfig{Context: r.KubernetesCluster}); err == nil {
					DeleteSocatProxyPod(kubeClient, r.SocatNamespace, r.SocatPodName)
				}
				r.SocatPodName = ""
				r.SocatNamespace = ""
			}
			
			if err != nil {
				// Check if this was an intentional stop
				if r.IntentionalStop {
					log.Printf("Port-forward stopped: cluster=%s, host=%s, ports=%d->%d", r.KubernetesCluster, r.RemoteHost, r.LocalPort, r.RemotePort)
				} else {
					log.Printf("Port-forward exited with error: cluster=%s, host=%s, ports=%d->%d, error=%v", r.KubernetesCluster, r.RemoteHost, r.LocalPort, r.RemotePort, err)
				}
			} else {
				log.Printf("Port-forward exited normally: cluster=%s, host=%s, ports=%d->%d", r.KubernetesCluster, r.RemoteHost, r.LocalPort, r.RemotePort)
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
		log.Printf("Row with ID %s not found. Available IDs: %v", id, func() []string {
			var ids []string
			for k := range g.rows {
				ids = append(ids, k)
			}
			return ids
		}())
		http.Error(w, "Proxy not found", http.StatusBadRequest)
		return
	}
	
	log.Printf("Disconnect request: cluster=%s, host=%s, ports=%d->%d", row.KubernetesCluster, row.RemoteHost, row.LocalPort, row.RemotePort)
	
	if !row.Connected {
		log.Printf("Row with ID %s is not connected", id)
		http.Error(w, "Proxy not connected", http.StatusBadRequest)
		return
	}
	
	// Kill the kubectl port-forward process
	if row.Process != nil {
		row.IntentionalStop = true // Mark as intentional stop
		if err := row.Process.Process.Kill(); err != nil {
			log.Printf("Error killing kubectl process: cluster=%s, host=%s, ports=%d->%d, error=%v", row.KubernetesCluster, row.RemoteHost, row.LocalPort, row.RemotePort, err)
		}
		row.Process = nil
	}
	
	// Clean up the socat pod
	if row.SocatPodName != "" {
		log.Printf("Cleaning up socat pod: %s in namespace %s", row.SocatPodName, row.SocatNamespace)
		kubeClient, err := GetKubernetesClient(KubeConfig{Context: row.KubernetesCluster})
		if err != nil {
			log.Printf("Failed to create Kubernetes client for cleanup: %v", err)
		} else {
			if err := DeleteSocatProxyPod(kubeClient, row.SocatNamespace, row.SocatPodName); err != nil {
				log.Printf("Error deleting socat pod %s: %v", row.SocatPodName, err)
			} else {
				log.Printf("Successfully deleted socat pod: %s", row.SocatPodName)
			}
		}
		row.SocatPodName = ""
		row.SocatNamespace = ""
	}
	
	row.Connected = false
	log.Printf("Successfully disconnected proxy: cluster=%s, host=%s, ports=%d->%d", row.KubernetesCluster, row.RemoteHost, row.LocalPort, row.RemotePort)
	
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

	g.mu.RLock()
	defer g.mu.RUnlock()

	// Convert current proxy rows to config format
	var configs []ProxyConfig
	for _, row := range g.rows {
		config := ProxyConfig{
			Name:              fmt.Sprintf("%s:%d", row.RemoteHost, row.LocalPort),
			KubernetesCluster: row.KubernetesCluster,
			RemoteHost:        row.RemoteHost,
			LocalPort:         row.LocalPort,
			RemotePort:        row.RemotePort,
		}
		configs = append(configs, config)
	}

	// Save to Viper and write to file
	viper.Set("proxy_configs", configs)
	
	var savedConfigFile string
	
	if !g.configFileLoaded {
		// No config file was initially loaded, use default location
		configFile := "./aproxymate.yaml"
		// Convert to absolute path for display and consistency
		absConfigFile, err := filepath.Abs(configFile)
		if err != nil {
			log.Printf("Error getting absolute path for config file: %v", err)
			absConfigFile = configFile // fallback to relative path
		}
		log.Printf("No config file was loaded on startup, saving to default location: %s", absConfigFile)
		savedConfigFile = absConfigFile
		
		// Force write using WriteConfigAs to ensure we get the correct filename
		err = viper.WriteConfigAs(configFile)
		if err != nil {
			log.Printf("Error saving config: %v", err)
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
			log.Printf("Error writing to existing config file: %v", err)
			http.Error(w, fmt.Sprintf("Failed to save configuration: %v", err), http.StatusInternalServerError)
			return
		}
		// Convert to absolute path for display consistency
		absConfigFile, err := filepath.Abs(configFile)
		if err != nil {
			log.Printf("Error getting absolute path for existing config file: %v", err)
			savedConfigFile = configFile // fallback to original path
		} else {
			savedConfigFile = absConfigFile
		}
	}

	log.Printf("Configuration saved successfully with %d proxy configs to %s", len(configs), savedConfigFile)

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
	nextSaveLocation := "./aproxymate.yaml"
	
	if g.configFileLoaded {
		configFile := viper.ConfigFileUsed()
		if configFile != "" {
			nextSaveLocation = configFile
		}
	}
	
	// Convert nextSaveLocation to absolute path for consistent display
	absNextSaveLocation, err := filepath.Abs(nextSaveLocation)
	if err != nil {
		log.Printf("Error getting absolute path for next save location: %v", err)
		absNextSaveLocation = nextSaveLocation // fallback to relative path
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"location":         location,
		"nextSaveLocation": absNextSaveLocation,
		"loaded":          fmt.Sprintf("%t", g.configFileLoaded),
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
				log.Printf("Process for ID %s has exited, updating status", id)
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
	
	log.Println("Cleaning up all active socat pods...")
	
	for _, row := range g.rows {
		if row.Connected && row.SocatPodName != "" {
			log.Printf("Cleaning up pod: cluster=%s, host=%s, ports=%d->%d, pod=%s", row.KubernetesCluster, row.RemoteHost, row.LocalPort, row.RemotePort, row.SocatPodName)
			
			// Kill the kubectl process
			if row.Process != nil {
				row.Process.Process.Kill()
			}
			
			// Delete the pod
			kubeClient, err := GetKubernetesClient(KubeConfig{Context: row.KubernetesCluster})
			if err != nil {
				log.Printf("Error creating client for cleanup: %v", err)
				continue
			}
			
			if err := DeleteSocatProxyPod(kubeClient, row.SocatNamespace, row.SocatPodName); err != nil {
				log.Printf("Error deleting pod %s: %v", row.SocatPodName, err)
			} else {
				log.Printf("Successfully cleaned up pod: %s", row.SocatPodName)
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
	absPath, err := filepath.Abs(configFile)
	if err != nil {
		log.Printf("Error getting absolute path for config file %s: %v", configFile, err)
		return configFile // fallback to original path
	}
	
	return absPath
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