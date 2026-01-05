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
	type candidateResult struct {
		url              string
		gpgURL           string
		debLine          string
		matchedPackages  []string
	}

	var result *candidateResult
	userProvidedURL := len(args) == 2

	for i, candidateURL := range candidateURLs {
		if verbose {
			fmt.Fprintf(os.Stderr, "Trying [%d/%d]: %s\n", i+1, len(candidateURLs), candidateURL)
		}

		resp, err := http.Get(candidateURL)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "  Failed to fetch: %v\n", err)
			}
			if userProvidedURL {
				fmt.Fprintf(os.Stderr, "Error: failed to fetch URL: %v\n", err)
				os.Exit(1)
			}
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "  Failed to read: %v\n", err)
			}
			if userProvidedURL {
				fmt.Fprintf(os.Stderr, "Error: failed to read response: %v\n", err)
				os.Exit(1)
			}
			continue
		}

		htmlContent := string(body)
		gpgURLs, debLines := finder.ParseRepoPage(htmlContent)

		if len(gpgURLs) > 0 && len(debLines) > 0 {
			if verbose {
				fmt.Fprintf(os.Stderr, "  Found GPG keys and deb sources!\n")
			}
		} else {
			// If no keys/sources found, look for install scripts
			if verbose {
				fmt.Fprintf(os.Stderr, "  No keys/sources on page, checking for install scripts...\n")
			}

			scriptURLs := findInstallScripts(htmlContent)
			for _, scriptURL := range scriptURLs {
				if verbose {
					fmt.Fprintf(os.Stderr, "  Fetching script: %s\n", scriptURL)
				}

				scriptResp, err := http.Get(scriptURL)
				if err != nil {
					continue
				}

				scriptBody, err := io.ReadAll(scriptResp.Body)
				scriptResp.Body.Close()
				if err != nil {
					continue
				}

				scriptGPGs, scriptDebs := parseInstallScript(string(scriptBody))
				gpgURLs = append(gpgURLs, scriptGPGs...)
				debLines = append(debLines, scriptDebs...)
				if len(gpgURLs) > 0 && len(debLines) > 0 {
					if verbose {
						fmt.Fprintf(os.Stderr, "  Found GPG keys and deb sources in script!\n")
					}
					break
				}
			}
		}

		// If we didn't find both keys and sources, try next candidate
		if len(gpgURLs) == 0 || len(debLines) == 0 {
			if verbose {
				fmt.Fprintf(os.Stderr, "  No keys/sources found\n")
			}
			if userProvidedURL {
				if len(gpgURLs) == 0 {
					fmt.Fprintf(os.Stderr, "Error: no GPG keys found\n")
				} else {
					fmt.Fprintf(os.Stderr, "Error: no deb sources found\n")
				}
				os.Exit(1)
			}
			continue
		}

		// Detect system for source filtering
		sysInfo, err := finder.DetectSystem()
		if err != nil && verbose {
			fmt.Fprintf(os.Stderr, "  Warning: could not detect system: %v\n", err)
		}

		// Try to match keys to sources (with system-aware filtering)
		var matches []finder.Match
		if sysInfo != nil {
			matches, err = finder.MatchKeysToSourcesWithSystem(gpgURLs, debLines, sysInfo)
		} else {
			matches, err = finder.MatchKeysToSources(gpgURLs, debLines)
		}
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "  %v\n", err)
			}
			if userProvidedURL {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			continue
		}

		// Use first match
		match := matches[0]

		if verbose {
			fmt.Fprintf(os.Stderr, "  Matched key: %s\n", match.GPGURL)
			fmt.Fprintf(os.Stderr, "  Matched source: %s\n", match.DebLine)
		}

		// Fetch package list
		if verbose {
			fmt.Fprintf(os.Stderr, "  Fetching package list...\n")
		}

		packages, err := finder.FetchPackageList(match.DebLine)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "  Error fetching packages: %v\n", err)
			}
			if userProvidedURL {
				fmt.Fprintf(os.Stderr, "Error fetching packages: %v\n", err)
				os.Exit(1)
			}
			continue
		}

		// Validate package glob matches at least one package
		matched, matchedPackages := validatePackageGlob(packageGlob, packages)
		if !matched {
			if verbose {
				fmt.Fprintf(os.Stderr, "  No packages matching '%s' found\n", packageGlob)
			}
			if userProvidedURL {
				fmt.Fprintf(os.Stderr, "Error: no packages matching '%s' found in repository\n", packageGlob)
				fmt.Fprintf(os.Stderr, "Available packages: %s\n", strings.Join(packages, ", "))
				os.Exit(1)
			}
			continue
		}

		// Success! We found a working repository
		result = &candidateResult{
			url:             candidateURL,
			gpgURL:          match.GPGURL,
			debLine:         match.DebLine,
			matchedPackages: matchedPackages,
		}
		break
	}

	// Check if we found a working candidate
	if result == nil {
		fmt.Fprintf(os.Stderr, "Error: no working repository found\n")
		os.Exit(1)
	}

	url = result.url
	if verbose {
		fmt.Fprintf(os.Stderr, "Found GPG key: %s\n", result.gpgURL)
		fmt.Fprintf(os.Stderr, "Found source: %s\n", result.debLine)
		fmt.Fprintf(os.Stderr, "Matched packages: %s\n", strings.Join(result.matchedPackages, ", "))
	}

	// Generate paths based on the package name the user searched for
	keyPath, _ := finder.GenerateKeyPath(result.gpgURL, packageGlob)
	sourcesEntry, sourcesFilename := finder.GenerateSourcesEntry(result.debLine, keyPath)

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
		fmt.Fprintf(os.Stderr, "Fetching key from %s\n", result.gpgURL)
	}
	keyData, err := finder.FetchKey(result.gpgURL)
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
		fmt.Fprintf(os.Stderr, "Configured repository for: %s\n", strings.Join(result.matchedPackages, ", "))
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
	// Strip glob wildcards for web search (they're for package matching, not search)
	searchTerm := strings.ReplaceAll(packageName, "*", "")
	searchTerm = strings.ReplaceAll(searchTerm, "?", "")
	searchTerm = strings.TrimSpace(searchTerm)

	query := fmt.Sprintf("%s debian ubuntu apt repository install", searchTerm)

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

	// Blacklist of low-quality tutorial sites and non-useful domains
	blacklist := []string{
		// Social media and general knowledge sites
		"wikipedia.org",
		"youtube.com",
		"facebook.com",
		"twitter.com",
		"reddit.com",
		// Generic tutorial sites (high SEO, low signal)
		"tecadmin.net",
		"tecmint.com",
		"linuxize.com",
		"linuxvox.com",
		"computingforgeeks.com",
		"linuxhint.com",
		"itsfoss.com",
		"howtogeek.com",
		"digitalocean.com/community/tutorials",
		// Generic documentation (not package-specific)
		"documentation.ubuntu.com",
		"ubuntu.com/tutorials",
		"debian.org/doc",
		// Corporate blogs (generic tutorials)
		"jumpcloud.com/blog",
		"operavps.com",
		// Other
		".pdf",
	}

	for _, result := range results {
		url := result.URL
		blacklisted := false
		for _, domain := range blacklist {
			if strings.Contains(url, domain) {
				blacklisted = true
				break
			}
		}
		if !blacklisted && !seen[url] {
			urls = append(urls, url)
			seen[url] = true
		}
	}

	return urls
}

// findInstallScripts extracts URLs to shell scripts from HTML
// Limit to 2 scripts max to avoid fetching too many false positives
func findInstallScripts(html string) []string {
	var scripts []string

	// Look for curl/wget commands with shell script URLs
	// Be strict: only match .sh files or /install.sh specifically
	patterns := []string{
		// curl/wget with .sh files
		`(?:curl|wget)[^\n]*?(https?://[^\s'"]+\.sh)`,
		// Direct links to .sh files
		`href=["'](https?://[^\s'"]+\.sh)["']`,
		// Specific /install.sh or /install (no other path components after)
		`(?:curl|wget)[^\n]*?(https?://[^\s'"]+/install\.sh)`,
		`(?:curl|wget)[^\n]*?(https?://[^\s'"]+/install)(?:\s|$|["'])`,
		`href=["'](https?://[^\s'"]+/install\.sh)["']`,
		`href=["'](https?://[^\s'"]+/install)["']`,
	}

	seen := make(map[string]bool)
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(html, -1)
		for _, match := range matches {
			if len(match) > 1 && !seen[match[1]] {
				// Additional validation: URL should not contain common non-script paths
				url := match[1]
				if strings.Contains(url, "/installer") ||
				   strings.Contains(url, "/installation") ||
				   strings.Contains(url, "/installing") {
					continue // Skip false positives like /installation-guide
				}
				scripts = append(scripts, url)
				seen[url] = true

				// Limit to 2 scripts max
				if len(scripts) >= 2 {
					return scripts
				}
			}
		}
	}

	return scripts
}

// parseInstallScript extracts GPG keys and deb sources from shell scripts
func parseInstallScript(script string) ([]string, []string) {
	var gpgURLs []string
	var debLines []string

	// Look for GPG key URLs (without variables)
	keyPatterns := []string{
		`https?://[^\s'"$]+\.(?:gpg|asc|key)`,
		`https?://[^\s'"$]+/gpg[^a-zA-Z]`,
		`https?://[^\s'"$]+\.noarmor\.gpg`,
	}

	seenKeys := make(map[string]bool)
	for _, pattern := range keyPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(script, -1)
		for _, match := range matches {
			if !seenKeys[match] {
				gpgURLs = append(gpgURLs, match)
				seenKeys[match] = true
			}
		}
	}

	// Look for URL patterns with shell variables and try to resolve them
	// Common patterns like: "https://pkgs.tailscale.com/$TRACK/$OS/$VERSION.noarmor.gpg"
	keyVarPatterns := []string{
		`https?://[^"'\s]*\$[A-Z_]+[^"'\s]*\.gpg`,
		`https?://[^"'\s]*\$[A-Z_]+[^"'\s]*\.asc`,
		`https?://[^"'\s]*\$[A-Z_]+[^"'\s]*\.key`,
	}

	// Common substitutions for Debian/Ubuntu systems
	substitutions := []map[string]string{
		{"TRACK": "stable", "OS": "ubuntu", "VERSION": "jammy"},
		{"TRACK": "stable", "OS": "ubuntu", "VERSION": "focal"},
		{"TRACK": "stable", "OS": "debian", "VERSION": "bookworm"},
		{"TRACK": "stable", "OS": "debian", "VERSION": "bullseye"},
	}

	for _, pattern := range keyVarPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(script, -1)
		for _, match := range matches {
			// Try each substitution
			for _, subst := range substitutions {
				resolved := match
				for varName, value := range subst {
					resolved = strings.ReplaceAll(resolved, "$"+varName, value)
				}
				if !seenKeys[resolved] {
					gpgURLs = append(gpgURLs, resolved)
					seenKeys[resolved] = true
				}
			}
		}
	}

	// Look for .list file URLs with variables that contain deb sources
	listPattern := regexp.MustCompile(`https?://[^"'\s]*\$[A-Z_]+[^"'\s]*\.list`)
	listMatches := listPattern.FindAllString(script, -1)

	seenListURLs := make(map[string]bool)
	seenDebs := make(map[string]bool)

	for _, match := range listMatches {
		// Try each substitution
		for _, subst := range substitutions {
			resolved := match
			for varName, value := range subst {
				resolved = strings.ReplaceAll(resolved, "$"+varName, value)
			}

			if seenListURLs[resolved] {
				continue
			}
			seenListURLs[resolved] = true

			// Fetch the .list file and extract deb lines
			resp, err := http.Get(resolved)
			if err != nil {
				continue
			}
			listBody, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				continue
			}

			// Parse deb lines from the .list file
			listDebPattern := regexp.MustCompile(`deb\s+(?:\[[^\]]+\]\s+)?https?://[^\s]+(?:\s+[a-zA-Z0-9][a-zA-Z0-9._-]*)+`)
			listDebMatches := listDebPattern.FindAllString(string(listBody), -1)
			for _, debMatch := range listDebMatches {
				line := strings.TrimRight(debMatch, `"';|`)
				line = regexp.MustCompile(`\s+`).ReplaceAllString(strings.TrimSpace(line), " ")
				if !seenDebs[line] {
					debLines = append(debLines, line)
					seenDebs[line] = true
				}
			}
		}
	}

	// Look for deb source lines directly in the script
	debPattern := regexp.MustCompile(`deb\s+(?:\[[^\]]+\]\s+)?https?://[^\s$]+(?:\s+[a-zA-Z0-9][a-zA-Z0-9._-]*)+`)
	matches := debPattern.FindAllString(script, -1)

	for _, match := range matches {
		// Clean up the line
		line := strings.TrimRight(match, `"';|`)
		line = regexp.MustCompile(`\s+`).ReplaceAllString(strings.TrimSpace(line), " ")
		if !seenDebs[line] {
			debLines = append(debLines, line)
			seenDebs[line] = true
		}
	}

	return gpgURLs, debLines
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
