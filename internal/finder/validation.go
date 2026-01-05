package finder

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

// MatchKeysToSources pairs GPG keys with deb sources by domain matching
func MatchKeysToSources(gpgURLs, debLines []string) ([]Match, error) {
	if len(gpgURLs) == 0 || len(debLines) == 0 {
		return nil, fmt.Errorf("no keys or sources found")
	}

	// Simple case: exactly one of each
	if len(gpgURLs) == 1 && len(debLines) == 1 {
		return []Match{{GPGURL: gpgURLs[0], DebLine: debLines[0]}}, nil
	}

	// Build domain map for sources (allow multiple sources per domain)
	sourcesByDomain := make(map[string][]string)
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
		sourcesByDomain[domain] = append(sourcesByDomain[domain], deb)
	}

	// Build domain map for keys
	keysByDomain := make(map[string][]string)
	for _, gpgURL := range gpgURLs {
		u, err := url.Parse(gpgURL)
		if err != nil {
			continue
		}
		domain := u.Host
		keysByDomain[domain] = append(keysByDomain[domain], gpgURL)
	}

	// Create matches
	var matches []Match
	for domain, sources := range sourcesByDomain {
		keys := keysByDomain[domain]
		if len(keys) == 0 {
			return nil, fmt.Errorf("no key found for domain %s", domain)
		}

		// If multiple keys but one source, that's ambiguous
		if len(keys) > 1 && len(sources) == 1 {
			return nil, fmt.Errorf("ambiguous: multiple keys for domain %s", domain)
		}

		// If one key and any number of sources, use the same key for all
		if len(keys) == 1 {
			for _, source := range sources {
				matches = append(matches, Match{
					GPGURL:  keys[0],
					DebLine: source,
				})
			}
			continue
		}

		// If multiple keys and multiple sources, try path-based matching
		if len(keys) > 1 && len(sources) > 1 {
			pathMatches, err := matchByPath(keys, sources)
			if err != nil {
				return nil, fmt.Errorf("ambiguous: multiple keys and sources for domain %s: %w", domain, err)
			}
			matches = append(matches, pathMatches...)
		}
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

// matchByPath attempts to match keys to sources by finding common keywords in paths
func matchByPath(keys, sources []string) ([]Match, error) {
	var matches []Match
	matched := make(map[string]bool)

	for _, key := range keys {
		// Extract path components from key URL
		keyURL, err := url.Parse(key)
		if err != nil {
			continue
		}
		keyPath := keyURL.Path

		// Try to find a matching source
		for _, source := range sources {
			if matched[source] {
				continue
			}

			// Extract distribution from deb line
			_, dist, _, err := parseDebLine(source)
			if err != nil {
				continue
			}

			// Check if the distribution name appears in the key path
			// e.g., "jammy" in "stable/ubuntu/jammy.noarmor.gpg"
			if regexp.MustCompile(`\b` + regexp.QuoteMeta(dist) + `\b`).MatchString(keyPath) {
				matches = append(matches, Match{
					GPGURL:  key,
					DebLine: source,
				})
				matched[source] = true
				break
			}
		}
	}

	// Check if all sources were matched
	if len(matches) != len(sources) {
		return nil, fmt.Errorf("could not match all sources (matched %d of %d)", len(matches), len(sources))
	}

	return matches, nil
}

// GenerateKeyPath creates the filesystem path for a key
func GenerateKeyPath(gpgURL, repoName string) (path, format string) {
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

// GenerateSourcesEntry creates a deb822 format sources file
func GenerateSourcesEntry(debLine, keyPath string) (entry, filename string) {
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

// CheckPrivileges returns true if running as root
func CheckPrivileges() bool {
	return os.Geteuid() == 0
}

// CheckDebianSystem returns true if running on Debian or derivative
func CheckDebianSystem() bool {
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

// CheckAptDirectories verifies required APT directories exist
func CheckAptDirectories() error {
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

// CheckConflicts checks if key or sources file already exist
func CheckConflicts(keyPath, sourcesFilename string) (keyExists, sourceExists bool) {
	_, err := os.Stat(keyPath)
	keyExists = err == nil

	sourcesPath := filepath.Join("/etc/apt/sources.list.d", sourcesFilename)
	_, err = os.Stat(sourcesPath)
	sourceExists = err == nil

	return keyExists, sourceExists
}
