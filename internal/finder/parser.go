package finder

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ParseRepoPage extracts GPG key URLs and deb source lines from HTML
func ParseRepoPage(html string) ([]string, []string) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return []string{}, []string{}
	}

	gpgURLs := findGPGKeys(doc)
	debLines := FindDebLines(doc, false, nil)

	return dedup(gpgURLs), dedup(debLines)
}

func findGPGKeys(doc *goquery.Document) []string {
	var urls []string
	text := doc.Text()

	// Pattern 1: URLs ending in .gpg, .asc, .key, or /gpg
	// Also supports OpenBuildService /public_key pattern
	patterns := []string{
		`https?://[^\s<>"']+\.(?:gpg|asc|key)`,
		`https?://[^\s<>"']+/gpg`,
		`https?://[^\s<>"']+\.noarmor\.gpg`,
		`https?://[^\s<>"']+/public_key`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(text, -1)
		for _, url := range matches {
			// Filter out false positives
			if isValidGPGURL(url) {
				urls = append(urls, url)
			}
		}
	}

	return urls
}

// isValidGPGURL filters out false positive GPG key URLs
func isValidGPGURL(url string) bool {
	// Filter bash brace expansions like "install.sh{,.asc"
	if strings.Contains(url, "{") || strings.Contains(url, "}") {
		return false
	}

	// Filter multi-line configs (yum repos, etc.)
	if strings.Contains(url, "\n") || strings.Contains(url, "\r") {
		return false
	}

	// Filter URLs with config-like syntax (=, [, ])
	if strings.Contains(url, "=") || strings.Contains(url, "[") {
		return false
	}

	// Filter URLs that look like install scripts with extensions
	// e.g., "install.sh.asc" is likely referring to a script signature, not a key
	if strings.Contains(url, "install.sh") {
		return false
	}

	return true
}

// FindDebLines extracts deb source lines from a goquery document
// If verbose is true and logger is provided, it logs .list file fetches
func FindDebLines(doc *goquery.Document, verbose bool, logger io.Writer) []string {
	var lines []string

	// Look for deb lines in code blocks and pre tags first (most reliable)
	doc.Find("code, pre").Each(func(i int, s *goquery.Selection) {
		text := s.Text()
		// Match deb line: deb [options] URL suite components
		// Components can be alphanumeric or "./" (OBS/repository root syntax)
		// Stop at newline, quote, pipe, or semicolon
		re := regexp.MustCompile(`deb\s+(?:\[[^\]]+\]\s+)?https?://[^\s]+(?:\s+(?:\./|[a-zA-Z0-9][a-zA-Z0-9._-]*))+`)
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
	re := regexp.MustCompile(`deb\s+(?:\[[^\]]+\]\s+)?https?://[^\s]+(?:\s+(?:\./|[a-zA-Z0-9][a-zA-Z0-9._-]*))+`)
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
			if verbose && logger != nil {
				logger.Write([]byte("Following .list file: " + listURL + "\n"))
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

// PPAInfo contains information about a Ubuntu PPA
type PPAInfo struct {
	Owner       string // PPA owner (e.g., "jlbarriere68")
	Name        string // PPA name (e.g., "noson-app")
	Fingerprint string // GPG key fingerprint
}

// FindPPAs extracts PPA references from HTML content
func FindPPAs(html string) []string {
	var ppas []string

	// Match ppa:owner/name pattern
	re := regexp.MustCompile(`ppa:([a-zA-Z0-9][a-zA-Z0-9._-]*)/([a-zA-Z0-9][a-zA-Z0-9._-]*)`)
	matches := re.FindAllStringSubmatch(html, -1)

	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) >= 3 {
			ppa := fmt.Sprintf("ppa:%s/%s", match[1], match[2])
			if !seen[ppa] {
				ppas = append(ppas, ppa)
				seen[ppa] = true
			}
		}
	}

	return ppas
}

// FetchPPAInfo fetches PPA information from Launchpad and returns GPG key URL and deb source
func FetchPPAInfo(ppa string, sysInfo *SystemInfo) (gpgURL string, debLine string, err error) {
	// Parse PPA syntax: ppa:owner/name
	re := regexp.MustCompile(`ppa:([^/]+)/(.+)`)
	matches := re.FindStringSubmatch(ppa)
	if len(matches) < 3 {
		return "", "", fmt.Errorf("invalid PPA format: %s", ppa)
	}

	owner := matches[1]
	name := matches[2]

	// Fetch Launchpad PPA page
	launchpadURL := fmt.Sprintf("https://launchpad.net/~%s/+archive/ubuntu/%s", owner, name)
	resp, err := http.Get(launchpadURL)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch PPA page: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read PPA page: %w", err)
	}

	// Extract key fingerprint from the page
	// Look for patterns like "4096R/FINGERPRINT" or just the fingerprint itself
	fingerprintRe := regexp.MustCompile(`(?:4096R?/)?([0-9A-F]{40})`)
	fingerprintMatches := fingerprintRe.FindAllStringSubmatch(string(body), -1)

	if len(fingerprintMatches) == 0 {
		return "", "", fmt.Errorf("could not find GPG key fingerprint on PPA page")
	}

	// Use the first fingerprint found
	fingerprint := fingerprintMatches[0][1]

	// Construct GPG key URL from Ubuntu keyserver
	gpgURL = fmt.Sprintf("https://keyserver.ubuntu.com/pks/lookup?op=get&search=0x%s", fingerprint)

	// Get Ubuntu codename (either real Ubuntu codename or mapped from Debian)
	ubuntuCodename, err := sysInfo.GetUbuntuCodename()
	if err != nil {
		return "", "", err
	}

	// Construct deb source line
	debLine = fmt.Sprintf("deb [arch=%s] http://ppa.launchpad.net/%s/%s/ubuntu %s main",
		sysInfo.Architecture, owner, name, ubuntuCodename)

	return gpgURL, debLine, nil
}

// PPAResult contains a GPG URL and deb source line from a PPA
type PPAResult struct {
	GPGURL  string
	DebLine string
}

// FetchPPAInfoWithFallback tries to fetch PPA info, and if on non-LTS Ubuntu,
// will return both the original codename version and an LTS fallback version
func FetchPPAInfoWithFallback(ppa string, sysInfo *SystemInfo) ([]PPAResult, error) {
	var results []PPAResult

	// Try with the actual codename first
	gpgURL, debLine, err := FetchPPAInfo(ppa, sysInfo)
	if err != nil {
		return nil, err
	}

	results = append(results, PPAResult{GPGURL: gpgURL, DebLine: debLine})

	// If on non-LTS Ubuntu, also provide LTS fallback
	if sysInfo.OSName == "ubuntu" {
		ltsCodename := GetNearestPastLTS(sysInfo.Codename)
		if ltsCodename != "" {
			// Create a temporary sysInfo with LTS codename
			ltsSysInfo := &SystemInfo{
				OSName:       sysInfo.OSName,
				Codename:     ltsCodename,
				Architecture: sysInfo.Architecture,
			}

			ltsGPGURL, ltsDebLine, err := FetchPPAInfo(ppa, ltsSysInfo)
			if err == nil {
				results = append(results, PPAResult{GPGURL: ltsGPGURL, DebLine: ltsDebLine})
			}
		}
	}

	return results, nil
}
