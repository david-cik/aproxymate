package lib

import (
	"os"
	"path/filepath"
)

const (
	ConfigFilename       = "aproxymate.yaml"
	HiddenConfigFilename = ".aproxymate.yaml"
)

// GetConfigSearchPaths returns the standard list of paths to search for config files,
// in priority order (highest to lowest priority)
func GetConfigSearchPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		// If we can't get home directory, just return local paths
		return []string{
			"./" + ConfigFilename,
			"./" + HiddenConfigFilename,
		}
	}

	return []string{
		// Current directory first (highest priority)
		"./" + ConfigFilename,
		"./" + HiddenConfigFilename,
		// Then home directory
		filepath.Join(home, ConfigFilename),
		filepath.Join(home, HiddenConfigFilename),
	}
}

// GetDefaultConfigPath returns the default path for creating new config files
// (in the user's home directory)
func GetDefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ConfigFilename), nil
}

// GetLocalConfigPath returns the local config path (in current directory)
func GetLocalConfigPath() string {
	return "./" + ConfigFilename
}

// GetLocalHiddenConfigPath returns the local hidden config path (in current directory)
func GetLocalHiddenConfigPath() string {
	return "./" + HiddenConfigFilename
}

// GetHomeConfigPath returns the config path in home directory
func GetHomeConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ConfigFilename), nil
}

// GetHomeHiddenConfigPath returns the hidden config path in home directory
func GetHomeHiddenConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, HiddenConfigFilename), nil
}

// FindExistingConfigFile searches for an existing config file in the standard paths
// Returns the path to the first found config file, or empty string if none found
func FindExistingConfigFile() string {
	for _, path := range GetConfigSearchPaths() {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}
