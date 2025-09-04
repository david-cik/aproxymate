package lib

import (
	"fmt"
)

// ValidateKubernetesCluster checks if the provided cluster exists in kubeconfig
func ValidateKubernetesCluster(clusterName string) (bool, error) {
	if clusterName == "" {
		return false, nil
	}

	clusters, err := GetKubernetesContexts("")
	if err != nil {
		return false, fmt.Errorf("failed to get available Kubernetes contexts: %w", err)
	}

	for _, cluster := range clusters {
		if cluster == clusterName {
			return true, nil
		}
	}

	return false, nil
}
