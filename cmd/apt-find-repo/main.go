package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"apt-find-repo/internal/finder"
)

var verbose bool

func main() {
	flag.BoolVar(&verbose, "v", false, "verbose output")
	flag.Parse()

	args := flag.Args()
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: apt-find-repo [-v] <package-glob> <url>\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "Finds a signed repository at the given URL and validates that it provides\n")
		fmt.Fprintf(os.Stderr, "packages matching the glob pattern. If run as root, configures apt to use\n")
		fmt.Fprintf(os.Stderr, "the repository. Otherwise, displays what would be configured.\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  apt-find-repo tailscale https://tailscale.com/kb/1039/install-ubuntu-2004\n")
		fmt.Fprintf(os.Stderr, "  sudo apt-find-repo 'tailscale*' https://tailscale.com/kb/1039/install-ubuntu-2004\n")
		os.Exit(1)
	}

	packageGlob := args[0]
	url := args[1]

	// Fetch and parse the page
	if verbose {
		fmt.Fprintf(os.Stderr, "Fetching %s...\n", url)
	}

	resp, err := http.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching URL: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
		os.Exit(1)
	}

	gpgURLs, debLines := finder.ParseRepoPage(string(body))

	if len(gpgURLs) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no GPG keys found\n")
		os.Exit(1)
	}

	if len(debLines) == 0 {
		fmt.Fprintf(os.Stderr, "Error: no deb sources found\n")
		os.Exit(1)
	}

	// Match keys to sources
	matches, err := finder.MatchKeysToSources(gpgURLs, debLines)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Use first match
	match := matches[0]

	if verbose {
		fmt.Fprintf(os.Stderr, "Found GPG key: %s\n", match.GPGURL)
		fmt.Fprintf(os.Stderr, "Found source: %s\n", match.DebLine)
	}

	// Fetch package list
	if verbose {
		fmt.Fprintf(os.Stderr, "Fetching package list...\n")
	}

	packages, err := finder.FetchPackageList(match.DebLine)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching packages: %v\n", err)
		os.Exit(1)
	}

	// Validate package glob matches at least one package
	matched, matchedPackages := validatePackageGlob(packageGlob, packages)
	if !matched {
		fmt.Fprintf(os.Stderr, "Error: no packages matching '%s' found in repository\n", packageGlob)
		fmt.Fprintf(os.Stderr, "Available packages: %s\n", strings.Join(packages, ", "))
		os.Exit(1)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Matched packages: %s\n", strings.Join(matchedPackages, ", "))
	}

	// Generate paths
	repoName := extractRepoName(url)
	keyPath, _ := finder.GenerateKeyPath(match.GPGURL, repoName)
	sourcesEntry, sourcesFilename := finder.GenerateSourcesEntry(match.DebLine, keyPath)

	// Check conflicts
	keyExists, sourceExists := finder.CheckConflicts(keyPath, sourcesFilename)
	if keyExists {
		fmt.Fprintf(os.Stderr, "Error: key already exists at %s\n", keyPath)
		os.Exit(1)
	}
	if sourceExists {
		sourcesPath := filepath.Join("/etc/apt/sources.list.d", sourcesFilename)
		fmt.Fprintf(os.Stderr, "Error: source already exists at %s\n", sourcesPath)
		os.Exit(1)
	}

	// Fetch key
	if verbose {
		fmt.Fprintf(os.Stderr, "Fetching key from %s\n", match.GPGURL)
	}
	keyData, err := finder.FetchKey(match.GPGURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching key: %v\n", err)
		os.Exit(1)
	}

	// Normalize key
	normalizedKey, err := finder.NormalizeKey(keyData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error processing key: %v\n", err)
		os.Exit(1)
	}

	// Write key
	if verbose {
		fmt.Fprintf(os.Stderr, "Writing %s\n", keyPath)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating keyrings directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(keyPath, normalizedKey, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing key: %v\n", err)
		os.Exit(1)
	}

	// Write sources file
	sourcesPath := filepath.Join("/etc/apt/sources.list.d", sourcesFilename)
	if verbose {
		fmt.Fprintf(os.Stderr, "Writing %s\n", sourcesPath)
	}
	if err := os.WriteFile(sourcesPath, []byte(sourcesEntry+"\n"), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing sources file: %v\n", err)
		os.Exit(1)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Configured repository for: %s\n", strings.Join(matchedPackages, ", "))
	}
}

// validatePackageGlob checks if the glob matches any packages
// Returns true and list of matched packages if there's a match
func validatePackageGlob(glob string, packages []string) (bool, []string) {
	var matched []string

	// Convert glob to regex pattern
	// Support simple wildcards: * matches anything, ? matches single char
	pattern := regexp.QuoteMeta(glob)
	pattern = strings.ReplaceAll(pattern, `\*`, ".*")
	pattern = strings.ReplaceAll(pattern, `\?`, ".")
	pattern = "^" + pattern + "$"

	re, err := regexp.Compile(pattern)
	if err != nil {
		// If regex fails, try exact match
		for _, pkg := range packages {
			if pkg == glob {
				return true, []string{pkg}
			}
		}
		return false, nil
	}

	for _, pkg := range packages {
		if re.MatchString(pkg) {
			matched = append(matched, pkg)
		}
	}

	return len(matched) > 0, matched
}

// extractRepoName tries to get a reasonable name from the URL
func extractRepoName(url string) string {
	// Try to extract from GitHub-style URLs
	re := regexp.MustCompile(`github\.com/[^/]+/([^/]+)`)
	if matches := re.FindStringSubmatch(url); len(matches) > 1 {
		return strings.TrimSuffix(matches[1], ".git")
	}

	// Fall back to domain
	re = regexp.MustCompile(`https?://([^/]+)`)
	if matches := re.FindStringSubmatch(url); len(matches) > 1 {
		return strings.Split(matches[1], ".")[0]
	}

	return "repo"
}
