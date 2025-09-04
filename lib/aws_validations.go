package lib

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Standard US regions commonly used
var standardUSRegions = []string{
	"us-east-1", // N. Virginia
	"us-east-2", // Ohio
	"us-west-1", // N. California
	"us-west-2", // Oregon
}

func ParseAWSProfiles() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(home, ".aws", "config")

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Return default profile if no config file exists
		return []string{"default"}, nil
	}

	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open AWS config file: %w", err)
	}
	defer file.Close()

	var profiles []string
	profilesMap := make(map[string]bool) // Use map to avoid duplicates
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Look for profile sections
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			// Remove brackets
			section := line[1 : len(line)-1]

			if section == "default" {
				profilesMap["default"] = true
			} else if strings.HasPrefix(section, "profile ") {
				// Extract profile name after "profile "
				profileName := strings.TrimSpace(section[8:])
				if profileName != "" {
					profilesMap[profileName] = true
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading AWS config file: %w", err)
	}

	// Convert map to sorted slice
	for profile := range profilesMap {
		profiles = append(profiles, profile)
	}

	// If no profiles found, add default
	if len(profiles) == 0 {
		profiles = append(profiles, "default")
	}

	// Sort profiles, but keep default first if it exists
	var sortedProfiles []string
	var otherProfiles []string

	for _, profile := range profiles {
		if profile == "default" {
			sortedProfiles = append([]string{"default"}, sortedProfiles...)
		} else {
			otherProfiles = append(otherProfiles, profile)
		}
	}

	// Sort other profiles alphabetically
	for i := 0; i < len(otherProfiles); i++ {
		for j := i + 1; j < len(otherProfiles); j++ {
			if otherProfiles[i] > otherProfiles[j] {
				otherProfiles[i], otherProfiles[j] = otherProfiles[j], otherProfiles[i]
			}
		}
	}

	sortedProfiles = append(sortedProfiles, otherProfiles...)

	return sortedProfiles, nil
}

// ValidateAWSProfile checks if the specified profile exists in the AWS config
func ValidateAWSProfile(profileName string) (bool, error) {
	if profileName == "" {
		return false, nil
	}

	profiles, err := ParseAWSProfiles()
	if err != nil {
		return false, err
	}

	for _, profile := range profiles {
		if profile == profileName {
			return true, nil
		}
	}

	return false, nil
}

// ValidateAWSRegion checks if the specified region is one of the standard US regions
func ValidateAWSRegion(region string) bool {
	if region == "" {
		return false
	}

	for _, standardRegion := range standardUSRegions {
		if region == standardRegion {
			return true
		}
	}

	return false
}
