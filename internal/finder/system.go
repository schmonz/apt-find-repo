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

// GetUbuntuCodename returns the Ubuntu codename for the system.
// For Ubuntu-based systems, returns the actual codename.
// For Debian systems, maps to the nearest equivalent Ubuntu version.
// Returns empty string if Debian codename is unknown (requires manual update).
func (s *SystemInfo) GetUbuntuCodename() (string, error) {
	if s.OSName == "ubuntu" {
		return s.Codename, nil
	}

	// Map Debian codename to Ubuntu equivalent for PPA compatibility
	debianToUbuntu := map[string]string{
		"trixie":   "noble",  // Debian 13 (testing) ≈ Ubuntu 24.04
		"bookworm": "jammy",  // Debian 12 ≈ Ubuntu 22.04
		"bullseye": "focal",  // Debian 11 ≈ Ubuntu 20.04
		"buster":   "bionic", // Debian 10 ≈ Ubuntu 18.04
		"stretch":  "xenial", // Debian 9 ≈ Ubuntu 16.04
	}

	if ubuntuCodename, ok := debianToUbuntu[s.Codename]; ok {
		return ubuntuCodename, nil
	}

	// No fallback - unknown Debian versions must be added manually
	return "", fmt.Errorf("unknown Debian codename '%s' - please update the mapping table", s.Codename)
}

// IsUbuntuLTS returns true if the Ubuntu codename is an LTS release
func IsUbuntuLTS(codename string) bool {
	ltsReleases := map[string]bool{
		"noble":   true, // 24.04 LTS
		"jammy":   true, // 22.04 LTS
		"focal":   true, // 20.04 LTS
		"bionic":  true, // 18.04 LTS
		"xenial":  true, // 16.04 LTS
		"trusty":  true, // 14.04 LTS
	}
	return ltsReleases[codename]
}

// GetNearestPastLTS returns the nearest past LTS release for non-LTS Ubuntu releases
// Returns empty string if already on LTS or unknown codename
func GetNearestPastLTS(codename string) string {
	// If already LTS, no fallback needed
	if IsUbuntuLTS(codename) {
		return ""
	}

	// Map non-LTS releases to their nearest past LTS
	// Ordered from newest to oldest
	nonLTSToLTS := map[string]string{
		"questing":  "noble",  // 25.10 → 24.04 LTS
		"plucky":    "noble",  // 25.04 → 24.04 LTS
		"oracular":  "noble",  // 24.10 → 24.04 LTS
		"mantic":    "jammy",  // 23.10 → 22.04 LTS
		"lunar":     "jammy",  // 23.04 → 22.04 LTS
		"kinetic":   "jammy",  // 22.10 → 22.04 LTS
		"impish":    "focal",  // 21.10 → 20.04 LTS
		"hirsute":   "focal",  // 21.04 → 20.04 LTS
		"groovy":    "focal",  // 20.10 → 20.04 LTS
		"eoan":      "bionic", // 19.10 → 18.04 LTS
		"disco":     "bionic", // 19.04 → 18.04 LTS
		"cosmic":    "bionic", // 18.10 → 18.04 LTS
	}

	if lts, ok := nonLTSToLTS[codename]; ok {
		return lts
	}

	// Unknown non-LTS, try the most recent LTS
	return "noble"
}
