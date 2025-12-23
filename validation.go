package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Match represents a paired GPG key and deb source line
type Match struct {
	GPGURL  string
	DebLine string
}

// matchKeysToSources pairs GPG keys with deb sources by domain matching
func matchKeysToSources(gpgURLs, debLines []string) ([]Match, error) {
	if len(gpgURLs) == 0 || len(debLines) == 0 {
		return nil, fmt.Errorf("no keys or sources found")
	}

	// Simple case: exactly one of each
	if len(gpgURLs) == 1 && len(debLines) == 1 {
		return []Match{{GPGURL: gpgURLs[0], DebLine: debLines[0]}}, nil
	}

	// Build domain map for sources
	sourceDomains := make(map[string]string)
	for _, deb := range debLines {
		debURL, _, _, err := parseDebLine(deb)
		if err != nil {
			continue
		}
		u, err := url.Parse(debURL)
		if err != nil {
			continue
		}
		domain := u.Host
		if _, exists := sourceDomains[domain]; exists {
			return nil, fmt.Errorf("ambiguous: multiple sources for domain %s", domain)
		}
		sourceDomains[domain] = deb
	}

	// Match keys to sources by domain
	var matches []Match
	keysByDomain := make(map[string][]string)

	for _, gpgURL := range gpgURLs {
		u, err := url.Parse(gpgURL)
		if err != nil {
			continue
		}
		domain := u.Host
		keysByDomain[domain] = append(keysByDomain[domain], gpgURL)
	}

	// Check for ambiguous matches
	for domain, keys := range keysByDomain {
		if len(keys) > 1 && sourceDomains[domain] != "" {
			return nil, fmt.Errorf("ambiguous: multiple keys for domain %s", domain)
		}
	}

	// Create matches
	for domain, deb := range sourceDomains {
		keys := keysByDomain[domain]
		if len(keys) == 0 {
			return nil, fmt.Errorf("no key found for domain %s", domain)
		}
		matches = append(matches, Match{
			GPGURL:  keys[0],
			DebLine: deb,
		})
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("could not match keys to sources")
	}

	// Sort for deterministic output
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].GPGURL < matches[j].GPGURL
	})

	return matches, nil
}

// generateKeyPath creates the filesystem path for a key
func generateKeyPath(gpgURL, repoName string) (path, format string) {
	// Sanitize repo name for filesystem
	safe := regexp.MustCompile(`[^a-z0-9-]`)
	name := safe.ReplaceAllString(strings.ToLower(repoName), "-")
	name = regexp.MustCompile(`-+`).ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")

	// Determine extension from URL
	ext := ".gpg"
	if strings.HasSuffix(gpgURL, ".asc") {
		ext = ".asc"
		format = "armored"
	} else {
		format = "binary"
	}

	path = filepath.Join("/etc/apt/keyrings", name+ext)
	return path, format
}

// generateSourcesEntry creates a deb822 format sources file
func generateSourcesEntry(debLine, keyPath string) (entry, filename string) {
	// Parse the deb line
	debURL, dist, comp, err := parseDebLine(debLine)
	if err != nil {
		return "", ""
	}

	// Extract additional components if present
	parts := strings.Fields(debLine)
	var components []string
	foundFirst := false
	for i, part := range parts {
		if foundFirst {
			components = append(components, part)
		}
		if i > 0 && (part == dist || strings.HasPrefix(part, "http")) {
			// Next parts after dist are components
			if part == dist {
				foundFirst = true
			}
		}
	}
	if len(components) == 0 {
		components = []string{comp}
	}

	// Extract architecture if present
	archRe := regexp.MustCompile(`\[([^\]]*arch=([^\s\]]+)[^\]]*)\]`)
	var arch string
	if matches := archRe.FindStringSubmatch(debLine); len(matches) > 2 {
		arch = matches[2]
	}

	// Build deb822 format
	var sb strings.Builder
	sb.WriteString("Types: deb\n")
	sb.WriteString(fmt.Sprintf("URIs: %s\n", debURL))
	sb.WriteString(fmt.Sprintf("Suites: %s\n", dist))
	sb.WriteString(fmt.Sprintf("Components: %s\n", strings.Join(components, " ")))
	if arch != "" {
		sb.WriteString(fmt.Sprintf("Architectures: %s\n", arch))
	}
	sb.WriteString(fmt.Sprintf("Signed-By: %s", keyPath))

	// Generate filename from key path
	base := filepath.Base(keyPath)
	filename = strings.TrimSuffix(base, filepath.Ext(base)) + ".sources"

	return sb.String(), filename
}

// checkPrivileges returns true if running as root
func checkPrivileges() bool {
	return os.Geteuid() == 0
}

// checkDebianSystem returns true if running on Debian or derivative
func checkDebianSystem() bool {
	// Check for dpkg
	if _, err := os.Stat("/usr/bin/dpkg"); err != nil {
		return false
	}
	// Check for apt
	if _, err := os.Stat("/usr/bin/apt"); err != nil {
		return false
	}
	return true
}

// checkAptDirectories verifies required APT directories exist
func checkAptDirectories() error {
	dirs := []string{
		"/etc/apt/keyrings",
		"/etc/apt/sources.list.d",
	}

	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("directory %s does not exist", dir)
			}
			return fmt.Errorf("cannot access %s: %v", dir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", dir)
		}
	}

	return nil
}

// checkConflicts checks if key or sources file already exist
func checkConflicts(keyPath, sourcesFilename string) (keyExists, sourceExists bool) {
	_, err := os.Stat(keyPath)
	keyExists = err == nil

	sourcesPath := filepath.Join("/etc/apt/sources.list.d", sourcesFilename)
	_, err = os.Stat(sourcesPath)
	sourceExists = err == nil

	return keyExists, sourceExists
}
