package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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
	if len(args) < 1 || len(args) > 2 {
		fmt.Fprintf(os.Stderr, "Usage: apt-find-repo [-v] <package-glob> [url]\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "Finds a signed repository and validates that it provides packages matching\n")
		fmt.Fprintf(os.Stderr, "the glob pattern, then configures apt to use it.\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "If URL is omitted, searches the web for the package and tries candidate\n")
		fmt.Fprintf(os.Stderr, "repositories automatically.\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "Examples:\n")
		fmt.Fprintf(os.Stderr, "  sudo apt-find-repo tailscale\n")
		fmt.Fprintf(os.Stderr, "  sudo apt-find-repo tailscale https://tailscale.com/kb/1039/install-ubuntu-2004\n")
		fmt.Fprintf(os.Stderr, "  sudo apt-find-repo 'tailscale*' https://tailscale.com/kb/...\n")
		os.Exit(1)
	}

	packageGlob := args[0]
	var url string
	var candidateURLs []string

	if len(args) == 2 {
		// Explicit URL provided
		url = args[1]
		candidateURLs = []string{url}
	} else {
		// Auto-discover: search the web
		if verbose {
			fmt.Fprintf(os.Stderr, "Searching for %s repositories...\n", packageGlob)
		}
		candidateURLs = searchForRepo(packageGlob)
		if len(candidateURLs) == 0 {
			fmt.Fprintf(os.Stderr, "Error: no candidate repositories found\n")
			os.Exit(1)
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "Found %d candidate URLs to try\n", len(candidateURLs))
		}
	}

	// Try each candidate URL until we find one that works
	var gpgURLs, debLines []string
	for i, candidateURL := range candidateURLs {
		if verbose {
			fmt.Fprintf(os.Stderr, "Trying [%d/%d]: %s\n", i+1, len(candidateURLs), candidateURL)
		}

		resp, err := http.Get(candidateURL)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "  Failed to fetch: %v\n", err)
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "  Failed to read: %v\n", err)
			}
			continue
		}

		gpgURLs, debLines = finder.ParseRepoPage(string(body))
		if len(gpgURLs) > 0 && len(debLines) > 0 {
			url = candidateURL
			if verbose {
				fmt.Fprintf(os.Stderr, "  Found GPG keys and deb sources!\n")
			}
			break
		}

		if verbose {
			fmt.Fprintf(os.Stderr, "  No keys/sources found\n")
		}
	}

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

// searchForRepo searches for repository URLs using ddgr or googler
func searchForRepo(packageName string) []string {
	query := fmt.Sprintf("%s debian ubuntu apt repository install", packageName)

	if verbose {
		fmt.Fprintf(os.Stderr, "Searching: %s\n", query)
	}

	// Try ddgr first
	cmd := exec.Command("ddgr", "--json", "--num", "15", "--np", query)
	output, err := cmd.Output()

	if err != nil {
		// Fall back to googler
		if verbose {
			fmt.Fprintf(os.Stderr, "ddgr failed, trying googler...\n")
		}
		cmd = exec.Command("googler", "--json", "--count", "15", "--np", query)
		output, err = cmd.Output()
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "Search failed: %v\n", err)
			}
			return nil
		}
	}

	// Parse JSON results
	var results []struct {
		URL string `json:"url"`
	}

	if err := json.Unmarshal(output, &results); err != nil {
		if verbose {
			fmt.Fprintf(os.Stderr, "Failed to parse search results: %v\n", err)
		}
		return nil
	}

	// Extract and filter URLs
	var urls []string
	seen := make(map[string]bool)

	for _, result := range results {
		url := result.URL
		// Filter for useful URLs
		if !seen[url] &&
		   !strings.Contains(url, "wikipedia.org") &&
		   !strings.Contains(url, "youtube.com") &&
		   !strings.Contains(url, "facebook.com") &&
		   !strings.Contains(url, "twitter.com") &&
		   !strings.Contains(url, ".pdf") {
			urls = append(urls, url)
			seen[url] = true
		}
	}

	return urls
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
