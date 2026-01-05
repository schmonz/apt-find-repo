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
	"github.com/PuerkitoBio/goquery"
)

var verbose bool

func main() {
	flag.BoolVar(&verbose, "v", false, "verbose output")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: apt-find-repo [-v] <url> [add]\n")
		os.Exit(1)
	}

	url := args[0]
	addMode := len(args) > 1 && args[1] == "add"

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

	gpgURLs, debLines := parseRepoPage(string(body))

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

	// For now, just use first match
	match := matches[0]

	if verbose {
		fmt.Fprintf(os.Stderr, "Found GPG key: %s\n", match.GPGURL)
		fmt.Fprintf(os.Stderr, "Found source: %s\n", match.DebLine)
	}

	if addMode {
		if !finder.CheckPrivileges() {
			fmt.Fprintf(os.Stderr, "Error: must run as root (use sudo)\n")
			os.Exit(1)
		}

		if !finder.CheckDebianSystem() {
			fmt.Fprintf(os.Stderr, "Error: not a Debian-based system\n")
			os.Exit(1)
		}

		if err := finder.CheckAptDirectories(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
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
			fmt.Fprintf(os.Stderr, "Done\n")
		}
	} else {
		// List packages
		if verbose {
			fmt.Fprintf(os.Stderr, "Fetching package list...\n")
		}

		packages, err := finder.FetchPackageList(match.DebLine)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching packages: %v\n", err)
			os.Exit(1)
		}

		for _, pkg := range packages {
			fmt.Println(pkg)
		}
	}
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

// parseRepoPage extracts GPG key URLs and deb source lines from HTML
func parseRepoPage(html string) ([]string, []string) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return []string{}, []string{}
	}

	gpgURLs := findGPGKeys(doc)
	debLines := findDebLines(doc)

	return dedup(gpgURLs), dedup(debLines)
}

func findGPGKeys(doc *goquery.Document) []string {
	var urls []string
	text := doc.Text()

	// Pattern 1: URLs ending in .gpg, .asc, .key, or /gpg
	patterns := []string{
		`https?://[^\s<>"']+\.(?:gpg|asc|key)`,
		`https?://[^\s<>"']+/gpg`,
		`https?://[^\s<>"']+\.noarmor\.gpg`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(text, -1)
		urls = append(urls, matches...)
	}

	return urls
}

func findDebLines(doc *goquery.Document) []string {
	var lines []string

	// Look for deb lines in code blocks and pre tags first (most reliable)
	doc.Find("code, pre").Each(func(i int, s *goquery.Selection) {
		text := s.Text()
		// Match deb line: deb [options] URL suite components
		// Stop at newline, quote, pipe, or semicolon
		re := regexp.MustCompile(`deb\s+(?:\[[^\]]+\]\s+)?https?://[^\s]+(?:\s+[a-zA-Z0-9][a-zA-Z0-9._-]*)+`)
		matches := re.FindAllString(text, -1)
		for _, match := range matches {
			// Clean trailing punctuation that might have snuck in
			match = strings.TrimRight(match, `"';|`)
			line := regexp.MustCompile(`\s+`).ReplaceAllString(strings.TrimSpace(match), " ")
			lines = append(lines, line)
		}
	})

	// If we found deb lines in code blocks, use those
	if len(lines) > 0 {
		return dedup(lines)
	}

	// Fall back to searching full text
	text := doc.Text()
	re := regexp.MustCompile(`deb\s+(?:\[[^\]]+\]\s+)?https?://[^\s]+(?:\s+[a-zA-Z0-9][a-zA-Z0-9._-]*)+`)
	matches := re.FindAllString(text, -1)

	for _, match := range matches {
		match = strings.TrimRight(match, `"';|`)
		line := regexp.MustCompile(`\s+`).ReplaceAllString(strings.TrimSpace(match), " ")
		lines = append(lines, line)
	}

	// Look for .list files in curl/wget commands that we should fetch
	listURLRe := regexp.MustCompile(`(?:curl|wget)[^\n]*?(https?://[^\s'"]+\.list)`)
	listMatches := listURLRe.FindAllStringSubmatch(text, -1)

	for _, match := range listMatches {
		if len(match) > 1 {
			listURL := match[1]
			if verbose {
				fmt.Fprintf(os.Stderr, "Following .list file: %s\n", listURL)
			}

			// Fetch the .list file
			resp, err := http.Get(listURL)
			if err != nil {
				continue
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				continue
			}

			// Parse deb lines from the fetched file
			listRe := regexp.MustCompile(`deb\s+(?:\[[^\]]+\]\s+)?https?://[^\s]+(?:\s+[a-zA-Z0-9][a-zA-Z0-9._-]*)+`)
			listDebLines := listRe.FindAllString(string(body), -1)
			for _, line := range listDebLines {
				line = strings.TrimRight(line, `"';|`)
				cleaned := regexp.MustCompile(`\s+`).ReplaceAllString(strings.TrimSpace(line), " ")
				lines = append(lines, cleaned)
			}
		}
	}

	return dedup(lines)
}

func dedup(s []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}
