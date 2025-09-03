package lib

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	log "aproxymate/lib/logger"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// KubeConfig represents configuration for Kubernetes connection
type KubeConfig struct {
	// KubeconfigPath is the path to kubeconfig file
	KubeconfigPath string
	// Context is the Kubernetes context to use
	Context string
}

// SocatProxyConfig represents configuration for a socat proxy pod
type SocatProxyConfig struct {
	// PodName is the name for the socat proxy pod
	PodName string
	// Namespace is the Kubernetes namespace to deploy the pod
	Namespace string
	// ListenPort is the port to listen on
	ListenPort int
	// RemoteHost is the target host to proxy to
	RemoteHost string
	// RemotePort is the target port to proxy to
	RemotePort int
}

// GetKubernetesClient creates a Kubernetes clientset using provided or default configuration
func GetKubernetesClient(config KubeConfig) (*kubernetes.Clientset, error) {
	// If no kubeconfig path provided, try to use default
	kubeconfigPath := config.KubeconfigPath
	if kubeconfigPath == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		} else {
			return nil, fmt.Errorf("unable to locate kubeconfig: home directory not found and no path provided")
		}
	}

	// Check if kubeconfig file exists
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("kubeconfig file not found at path: %s", kubeconfigPath)
	}

	// Build config from the kubeconfig file
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = kubeconfigPath

	configOverrides := &clientcmd.ConfigOverrides{}
	if config.Context != "" {
		configOverrides.CurrentContext = config.Context
	}

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	clientConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client config: %w", err)
	}

	// Create the clientset
	clientset, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return clientset, nil
}

// GetKubernetesClientConfig creates a Kubernetes client config using provided or default configuration
func GetKubernetesClientConfig(config KubeConfig) (*rest.Config, error) {
	// If no kubeconfig path provided, try to use default
	kubeconfigPath := config.KubeconfigPath
	if kubeconfigPath == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		} else {
			return nil, fmt.Errorf("unable to locate kubeconfig: home directory not found and no path provided")
		}
	}

	// Check if kubeconfig file exists
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("kubeconfig file not found at path: %s", kubeconfigPath)
	}

	// Build config from the kubeconfig file
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = kubeconfigPath

	configOverrides := &clientcmd.ConfigOverrides{}
	if config.Context != "" {
		configOverrides.CurrentContext = config.Context
	}

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	clientConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client config: %w", err)
	}

	return clientConfig, nil
}

// GetKubernetesContexts returns a list of available Kubernetes contexts from kubeconfig
func GetKubernetesContexts(kubeconfigPath string) ([]string, error) {
	// If no kubeconfig path provided, try to use default
	if kubeconfigPath == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		} else {
			return nil, fmt.Errorf("unable to locate kubeconfig: home directory not found and no path provided")
		}
	}

	// Check if kubeconfig file exists
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("kubeconfig file not found at path: %s", kubeconfigPath)
	}

	// Load the kubeconfig file
	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Extract context names
	var contexts []string
	for contextName := range config.Contexts {
		contexts = append(contexts, contextName)
	}

	return contexts, nil
}

// GetCurrentKubernetesContext returns the current default context from kubeconfig
func GetCurrentKubernetesContext(kubeconfigPath string) (string, error) {
	// If no kubeconfig path provided, try to use default
	if kubeconfigPath == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfigPath = filepath.Join(home, ".kube", "config")
		} else {
			return "", fmt.Errorf("unable to locate kubeconfig: home directory not found and no path provided")
		}
	}

	// Check if kubeconfig file exists
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return "", fmt.Errorf("kubeconfig file not found at path: %s", kubeconfigPath)
	}

	// Load the kubeconfig file
	config, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	return config.CurrentContext, nil
}

// PromptForKubernetesCluster prompts the user to select a Kubernetes cluster when none is specified
func PromptForKubernetesCluster() (string, error) {
	log.Debug("No Kubernetes cluster specified, looking up available clusters")

	contexts, err := GetKubernetesContexts("")
	if err != nil {
		return "", fmt.Errorf("failed to get available Kubernetes contexts: %w", err)
	}

	if len(contexts) == 0 {
		return "", fmt.Errorf("no Kubernetes contexts found in kubeconfig. Please ensure kubectl is configured with at least one cluster")
	}

	// Get current context as a default suggestion
	currentContext, err := GetCurrentKubernetesContext("")
	if err != nil {
		log.Debug("Could not determine current context", "error", err)
		currentContext = ""
	}

	fmt.Println("\n🔍 No Kubernetes cluster specified in configuration.")
	fmt.Printf("Found %d available cluster(s) in your kubeconfig:\n\n", len(contexts))

	// Display available contexts with numbering
	for i, context := range contexts {
		prefix := fmt.Sprintf("%d.", i+1)
		if context == currentContext {
			fmt.Printf("  %s %s (current)\n", prefix, context)
		} else {
			fmt.Printf("  %s %s\n", prefix, context)
		}
	}

	// If there's only one context, use it automatically
	if len(contexts) == 1 {
		selectedContext := contexts[0]
		fmt.Printf("\nAutomatically selecting the only available cluster: %s\n", selectedContext)
		log.Debug("Automatically selected single available cluster", "cluster", selectedContext)
		return selectedContext, nil
	}

	// If there's a current context, suggest it as default
	if currentContext != "" {
		fmt.Printf("\nPress Enter to use the current context (%s), or enter a cluster name/number: ", currentContext)
	} else {
		fmt.Print("\nEnter the cluster name or number to use: ")
	}

	// Read user input
	var input string
	fmt.Scanln(&input)

	// If empty input and we have a current context, use it
	if input == "" && currentContext != "" {
		log.Debug("Using current context as default", "cluster", currentContext)
		return currentContext, nil
	}

	// If empty input and no current context, prompt again
	if input == "" {
		return "", fmt.Errorf("no cluster selected. Please specify a cluster name or number")
	}

	// Check if input is a number
	if num, err := strconv.Atoi(input); err == nil {
		if num < 1 || num > len(contexts) {
			return "", fmt.Errorf("invalid selection: %d. Please choose a number between 1 and %d", num, len(contexts))
		}
		selectedContext := contexts[num-1]
		log.Debug("Selected cluster by number", "number", num, "cluster", selectedContext)
		return selectedContext, nil
	}

	// Check if input matches a context name
	for _, context := range contexts {
		if context == input {
			log.Debug("Selected cluster by name", "cluster", context)
			return context, nil
		}
	}

	return "", fmt.Errorf("cluster '%s' not found. Available clusters: %v", input, contexts)
}

// CreateSocatProxyPod creates a pod running socat to proxy traffic
func CreateSocatProxyPod(clientset *kubernetes.Clientset, config SocatProxyConfig) (*corev1.Pod, error) {
	// Default to "default" namespace if not specified
	namespace := config.Namespace
	if namespace == "" {
		namespace = "default"
	}

	// Default pod name if not provided
	podName := config.PodName
	if podName == "" {
		podName = fmt.Sprintf("socat-proxy-%d", time.Now().Unix())
	}

	// Validate required fields
	if config.RemoteHost == "" {
		return nil, fmt.Errorf("remote host is required")
	}
	if config.RemotePort <= 0 {
		return nil, fmt.Errorf("valid remote port is required")
	}
	if config.ListenPort <= 0 {
		return nil, fmt.Errorf("valid listen port is required")
	}

	// Create socat command
	socatCommand := fmt.Sprintf("TCP-LISTEN:%d,fork", config.ListenPort)
	socatTarget := fmt.Sprintf("TCP:%s:%d", config.RemoteHost, config.RemotePort)

	// Get current user for labeling
	currentUser := "unknown"
	if u := os.Getenv("USER"); u != "" {
		currentUser = u
	} else if u := os.Getenv("USERNAME"); u != "" {
		currentUser = u
	}

	// Define pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                "aproxymate",
				"component":          "socat-proxy",
				"created-by":         "aproxymate",
				"user":               currentUser,
				"aproxymate.managed": "true",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "socat",
					Image:   "alpine/socat",
					Command: []string{"socat"},
					Args:    []string{socatCommand, socatTarget},
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: int32(config.ListenPort),
							Protocol:      corev1.ProtocolTCP,
						},
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("50m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	// Create the pod
	createdPod, err := clientset.CoreV1().Pods(namespace).Create(
		context.Background(),
		pod,
		metav1.CreateOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create socat proxy pod: %w", err)
	}

	return createdPod, nil
}

// WaitForPodRunning waits for a pod to reach Running state with timeout
func WaitForPodRunning(clientset *kubernetes.Clientset, namespace, podName string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Poll every second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for pod %s to be running", podName)
		case <-ticker.C:
			pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("error getting pod %s: %w", podName, err)
			}

			if pod.Status.Phase == corev1.PodRunning {
				return nil
			}

			if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
				return fmt.Errorf("pod %s is in phase %s, not running", podName, pod.Status.Phase)
			}
		}
	}
}

// DeleteSocatProxyPod deletes a socat proxy pod by name
func DeleteSocatProxyPod(clientset *kubernetes.Clientset, namespace, podName string) error {
	err := clientset.CoreV1().Pods(namespace).Delete(
		context.Background(),
		podName,
		metav1.DeleteOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to delete socat proxy pod: %w", err)
	}
	return nil
}

// CleanupOrphanedAproxymatePodsForUser cleans up any orphaned aproxymate pods for the current user
func CleanupOrphanedAproxymatePodsForUser(clientset *kubernetes.Clientset, namespace string) error {
	if namespace == "" {
		namespace = "default"
	}

	// Get current user
	currentUser := "unknown"
	if u := os.Getenv("USER"); u != "" {
		currentUser = u
	} else if u := os.Getenv("USERNAME"); u != "" {
		currentUser = u
	}

	// List all aproxymate pods for this user
	listOptions := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("aproxymate.managed=true,user=%s", currentUser),
	}

	pods, err := clientset.CoreV1().Pods(namespace).List(context.Background(), listOptions)
	if err != nil {
		return fmt.Errorf("failed to list aproxymate pods: %w", err)
	}

	// Only log if there are orphaned pods to clean up
	if len(pods.Items) > 0 {
		log.Debug("Found orphaned aproxymate pods for cleanup", "user", currentUser, "count", len(pods.Items))
	}

	// Delete each pod
	for _, pod := range pods.Items {
		log.Debug("Cleaning up orphaned pod", "pod", pod.Name, "user", currentUser)
		err := clientset.CoreV1().Pods(namespace).Delete(
			context.Background(),
			pod.Name,
			metav1.DeleteOptions{},
		)
		if err != nil {
			log.Warn("Failed to delete orphaned pod", "pod", pod.Name, "error", err)
		} else {
			log.Debug("Successfully deleted orphaned pod", "pod", pod.Name)
		}
	}

	return nil
}

// CleanupAllOrphanedAproxymatePodsInNamespace cleans up all aproxymate pods in a namespace
func CleanupAllOrphanedAproxymatePodsInNamespace(clientset *kubernetes.Clientset, namespace string) error {
	if namespace == "" {
		namespace = "default"
	}

	// List all aproxymate pods
	listOptions := metav1.ListOptions{
		LabelSelector: "aproxymate.managed=true",
	}

	pods, err := clientset.CoreV1().Pods(namespace).List(context.Background(), listOptions)
	if err != nil {
		return fmt.Errorf("failed to list aproxymate pods: %w", err)
	}

	// Only log if there are orphaned pods to clean up
	if len(pods.Items) > 0 {
		log.Debug("Found orphaned aproxymate pods for cleanup", "namespace", namespace, "count", len(pods.Items))
	}

	// Delete each pod
	for _, pod := range pods.Items {
		log.Debug("Cleaning up orphaned pod", "pod", pod.Name, "namespace", namespace)
		err := clientset.CoreV1().Pods(namespace).Delete(
			context.Background(),
			pod.Name,
			metav1.DeleteOptions{},
		)
		if err != nil {
			log.Warn("Failed to delete orphaned pod", "pod", pod.Name, "namespace", namespace, "error", err)
		} else {
			log.Debug("Successfully deleted orphaned pod", "pod", pod.Name, "namespace", namespace)
		}
	}

	return nil
}
