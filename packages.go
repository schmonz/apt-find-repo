package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// parseDebLine extracts URL, distribution, and component from a deb source line
func parseDebLine(debLine string) (url, dist, component string, err error) {
	// Remove leading "deb" or "deb-src"
	line := strings.TrimPrefix(debLine, "deb-src")
	line = strings.TrimPrefix(line, "deb")
	line = strings.TrimSpace(line)

	// Strip options like [arch=amd64 signed-by=...]
	re := regexp.MustCompile(`^\[([^\]]+)\]\s+`)
	line = re.ReplaceAllString(line, "")

	// Parse: URL DIST COMPONENT [COMPONENT...]
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("invalid deb line format")
	}

	url = parts[0]
	dist = parts[1]
	component = parts[2] // take first component

	return url, dist, component, nil
}

// fetchPackageList retrieves the list of packages from a deb repository
func fetchPackageList(debLine string) ([]string, error) {
	url, dist, comp, err := parseDebLine(debLine)
	if err != nil {
		return nil, err
	}

	// Try common architectures
	arches := []string{"amd64", "all"}

	for _, arch := range arches {
		// Try Packages.gz first (smaller), then uncompressed
		urls := []string{
			fmt.Sprintf("%s/dists/%s/%s/binary-%s/Packages.gz", url, dist, comp, arch),
			fmt.Sprintf("%s/dists/%s/%s/binary-%s/Packages", url, dist, comp, arch),
		}

		for _, pkgURL := range urls {
			packages, err := fetchAndParsePackages(pkgURL)
			if err == nil && len(packages) > 0 {
				return packages, nil
			}
		}
	}

	return []string{}, nil
}

// fetchAndParsePackages downloads and parses a Packages file
func fetchAndParsePackages(url string) ([]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var reader io.Reader = resp.Body

	// Decompress if gzipped
	if strings.HasSuffix(url, ".gz") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		reader = gz
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return parsePackagesFile(data), nil
}

// parsePackagesFile extracts package names from Debian Packages file format
func parsePackagesFile(data []byte) []string {
	var packages []string
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(bytes.NewReader(data))

	for scanner.Scan() {
		line := scanner.Text()

		// Package stanzas are separated by blank lines
		// We only care about "Package:" fields
		if strings.HasPrefix(line, "Package:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				pkgName := strings.TrimSpace(parts[1])
				if !seen[pkgName] {
					seen[pkgName] = true
					packages = append(packages, pkgName)
				}
			}
		}
	}

	return packages
}
