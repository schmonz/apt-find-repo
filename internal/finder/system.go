package finder

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// SystemInfo contains information about the running system
type SystemInfo struct {
	OSName       string // "ubuntu" or "debian"
	Codename     string // "jammy", "focal", "bookworm", etc.
	Architecture string // "amd64", "arm64", etc.
}

// DetectSystem detects the running Debian/Ubuntu system
func DetectSystem() (*SystemInfo, error) {
	info := &SystemInfo{}

	// Detect architecture
	arch, err := detectArchitecture()
	if err != nil {
		return nil, fmt.Errorf("failed to detect architecture: %w", err)
	}
	info.Architecture = arch

	// Detect OS name and codename from /etc/os-release
	osName, codename, err := detectOSRelease()
	if err != nil {
		return nil, fmt.Errorf("failed to detect OS: %w", err)
	}
	info.OSName = osName
	info.Codename = codename

	return info, nil
}

// detectArchitecture uses dpkg to detect system architecture
func detectArchitecture() (string, error) {
	cmd := exec.Command("dpkg", "--print-architecture")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// detectOSRelease parses /etc/os-release to get OS name and codename
func detectOSRelease() (osName, codename string, err error) {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			osName = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		} else if strings.HasPrefix(line, "VERSION_CODENAME=") {
			codename = strings.Trim(strings.TrimPrefix(line, "VERSION_CODENAME="), "\"")
		} else if strings.HasPrefix(line, "UBUNTU_CODENAME=") {
			// Ubuntu uses UBUNTU_CODENAME, prefer it over VERSION_CODENAME
			codename = strings.Trim(strings.TrimPrefix(line, "UBUNTU_CODENAME="), "\"")
		}
	}

	if err := scanner.Err(); err != nil {
		return "", "", err
	}

	if osName == "" || codename == "" {
		return "", "", fmt.Errorf("could not detect OS name or codename")
	}

	// Normalize OS names
	switch osName {
	case "ubuntu", "pop", "neon", "zorin", "tuxedo":
		osName = "ubuntu"
	case "debian", "raspbian":
		osName = "debian"
	}

	return osName, codename, nil
}
