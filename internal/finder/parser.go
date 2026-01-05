package finder

import (
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

// FindDebLines extracts deb source lines from a goquery document
// If verbose is true and logger is provided, it logs .list file fetches
func FindDebLines(doc *goquery.Document, verbose bool, logger io.Writer) []string {
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
