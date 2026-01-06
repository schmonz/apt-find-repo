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
	return MatchKeysToSourcesWithSystem(gpgURLs, debLines, nil)
}

// MatchKeysToSourcesWithSystem pairs GPG keys with deb sources, preferring sources that match the system
func MatchKeysToSourcesWithSystem(gpgURLs, debLines []string, sysInfo *SystemInfo) ([]Match, error) {
	if len(gpgURLs) == 0 || len(debLines) == 0 {
		return nil, fmt.Errorf("no keys or sources found")
	}

	// Filter and rank sources by system match if system info provided
	if sysInfo != nil {
		debLines = FilterSourcesForSystem(debLines, sysInfo)
	}

	// Simple case: exactly one of each
	if len(gpgURLs) == 1 && len(debLines) == 1 {
		return []Match{{GPGURL: gpgURLs[0], DebLine: debLines[0]}}, nil
	}

	// Special handling for PPAs: keys from keyserver.ubuntu.com match sources from ppa.launchpad.net
	// PPAs are added in pairs, so we can match by index
	var matches []Match
	ppaKeyIndices := []int{}
	ppaSourceIndices := []int{}

	for i, gpgURL := range gpgURLs {
		if strings.Contains(gpgURL, "keyserver.ubuntu.com") {
			ppaKeyIndices = append(ppaKeyIndices, i)
		}
	}

	for i, debLine := range debLines {
		if strings.Contains(debLine, "ppa.launchpad.net") {
			ppaSourceIndices = append(ppaSourceIndices, i)
		}
	}

	// Match PPAs by index (they're added in pairs)
	minPPA := len(ppaKeyIndices)
	if len(ppaSourceIndices) < minPPA {
		minPPA = len(ppaSourceIndices)
	}

	for i := 0; i < minPPA; i++ {
		matches = append(matches, Match{
			GPGURL:  gpgURLs[ppaKeyIndices[i]],
			DebLine: debLines[ppaSourceIndices[i]],
		})
	}

	// If we matched all keys and sources with PPAs, return
	if len(matches) == len(gpgURLs) && len(matches) == len(debLines) {
		return matches, nil
	}

	// For mixed cases, filter out already-matched PPAs and continue with domain matching
	remainingKeys := []string{}
	remainingSources := []string{}
	matchedKeyIndices := make(map[int]bool)
	matchedSourceIndices := make(map[int]bool)

	for _, idx := range ppaKeyIndices[:minPPA] {
		matchedKeyIndices[idx] = true
	}
	for _, idx := range ppaSourceIndices[:minPPA] {
		matchedSourceIndices[idx] = true
	}

	for i, key := range gpgURLs {
		if !matchedKeyIndices[i] {
			remainingKeys = append(remainingKeys, key)
		}
	}

	for i, source := range debLines {
		if !matchedSourceIndices[i] {
			remainingSources = append(remainingSources, source)
		}
	}

	// If no remaining keys/sources, we're done
	if len(remainingKeys) == 0 || len(remainingSources) == 0 {
		if len(matches) > 0 {
			return matches, nil
		}
		return nil, fmt.Errorf("no matching keys and sources found")
	}

	// Continue with domain-based matching for non-PPA keys/sources
	gpgURLs = remainingKeys
	debLines = remainingSources

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

	// Add domain-based matches to existing PPA matches
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

// FilterSourcesForSystem filters deb sources to prefer those matching the running system
func FilterSourcesForSystem(sources []string, sysInfo *SystemInfo) []string {
	if sysInfo == nil || len(sources) == 0 {
		return sources
	}

	type scoredSource struct {
		source string
		score  int
	}

	scored := make([]scoredSource, 0, len(sources))

	for _, source := range sources {
		score := scoreSource(source, sysInfo)
		// Only include sources with non-negative scores
		if score >= 0 {
			scored = append(scored, scoredSource{source, score})
		}
	}

	// Sort by score (highest first)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Return sorted sources
	result := make([]string, len(scored))
	for i, s := range scored {
		result[i] = s.source
	}

	return result
}

// scoreSource assigns a score to a deb source based on system match
// Higher score = better match
func scoreSource(source string, sysInfo *SystemInfo) int {
	score := 0

	// Parse the source line
	_, dist, _, err := parseDebLine(source)
	if err != nil {
		return -1000 // Invalid source
	}

	sourceLower := strings.ToLower(source)
	dist = strings.ToLower(dist)

	// Check architecture match
	if strings.Contains(sourceLower, "arch="+sysInfo.Architecture) ||
		strings.Contains(sourceLower, "$(dpkg --print-architecture)") {
		score += 100
	} else if strings.Contains(sourceLower, "arch=") {
		// Has arch specified but doesn't match ours
		archPattern := regexp.MustCompile(`arch=([a-z0-9]+)`)
		if match := archPattern.FindStringSubmatch(sourceLower); len(match) > 1 {
			if match[1] != sysInfo.Architecture && match[1] != "all" {
				return -1000 // Wrong architecture, skip
			}
		}
	}

	// Check distribution/codename match
	if dist == sysInfo.Codename {
		score += 1000 // Exact codename match (highest priority)
	} else if dist == sysInfo.OSName {
		score += 500 // OS name match (ubuntu, debian)
	} else if dist == "stable" || dist == "any" {
		score += 100 // Generic stable/any
	} else if dist == "testing" || dist == "unstable" || dist == "sid" {
		// Debian suite names - valid but not preferred
		if sysInfo.OSName == "debian" {
			score += 50 // Valid for debian
		} else {
			return -1000 // These are debian-only
		}
	} else {
		// Check if it's a different codename for same OS
		knownCodenames := map[string]string{
			"jammy":    "ubuntu",
			"focal":    "ubuntu",
			"noble":    "ubuntu",
			"mantic":   "ubuntu",
			"bookworm": "debian",
			"bullseye": "debian",
			"buster":   "debian",
			"trixie":   "debian",
		}
		if knownOS, ok := knownCodenames[dist]; ok {
			if knownOS == sysInfo.OSName {
				score += 50 // Same OS but different version
			} else {
				return -1000 // Different OS, skip
			}
		}
	}

	// Prefer stable over testing/unstable (but don't filter out completely)
	if strings.Contains(sourceLower, "stable") {
		score += 20
	}
	if strings.Contains(sourceLower, "main") && !strings.Contains(sourceLower, "testing") {
		score += 10
	}
	if strings.Contains(sourceLower, "testing") {
		score -= 10 // Deprioritize but don't filter out
	}
	if strings.Contains(sourceLower, "unstable") {
		score -= 20 // Deprioritize but don't filter out
	}

	return score
}
