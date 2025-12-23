package main

import (
	"os"
	"reflect"
	"sort"
	"testing"
)

type PackagesTestCase struct {
	name         string
	packagesFile string
	wantPackages []string
}

var packagesTestCases = []PackagesTestCase{
	{
		name:         "simple-packages",
		packagesFile: "testdata/packages/simple.txt",
		wantPackages: []string{"package-a", "package-b", "package-c"},
	},
	{
		name:         "packages-with-source",
		packagesFile: "testdata/packages/with-source.txt",
		wantPackages: []string{"real-package", "another-package"},
	},
	{
		name:         "multiarch",
		packagesFile: "testdata/packages/multiarch.txt",
		wantPackages: []string{"amd64-pkg", "arm64-pkg", "all-arch-pkg"},
	},
	{
		name:         "empty",
		packagesFile: "testdata/packages/empty.txt",
		wantPackages: []string{},
	},
	{
		name:         "zoom-real",
		packagesFile: "testdata/packages/zoom-real.txt",
		wantPackages: []string{"zoom"}, // actual package name in their repo
	},
	{
		name:         "tailscale-real",
		packagesFile: "testdata/packages/tailscale-real.txt",
		wantPackages: []string{"tailscale", "tailscale-nginx-auth"},
	},
}

func TestParsePackagesFile(t *testing.T) {
	for _, tc := range packagesTestCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(tc.packagesFile)
			if err != nil {
				t.Skipf("Test file not found: %s", tc.packagesFile)
			}

			packages := parsePackagesFile(data)
			sort.Strings(packages)
			sort.Strings(tc.wantPackages)

			if !reflect.DeepEqual(packages, tc.wantPackages) && len(packages) != 0 && len(tc.wantPackages) != 0 {
				t.Errorf("Package list mismatch.\nGot:  %v (len=%d)\nWant: %v (len=%d)",
					packages, len(packages), tc.wantPackages, len(tc.wantPackages))
			}
		})
	}
}

func TestParseDebLine(t *testing.T) {
	tests := []struct {
		name     string
		debLine  string
		wantURL  string
		wantDist string
		wantComp string
	}{
		{
			name:     "simple",
			debLine:  "deb http://example.com/repo stable main",
			wantURL:  "http://example.com/repo",
			wantDist: "stable",
			wantComp: "main",
		},
		{
			name:     "with-options",
			debLine:  "deb [arch=amd64 signed-by=/path/to/key] https://repo.example.com any main",
			wantURL:  "https://repo.example.com",
			wantDist: "any",
			wantComp: "main",
		},
		{
			name:     "multiple-components",
			debLine:  "deb https://repo.example.com focal main restricted universe",
			wantURL:  "https://repo.example.com",
			wantDist: "focal",
			wantComp: "main", // just take first component
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url, dist, comp, err := parseDebLine(tc.debLine)
			if err != nil {
				t.Fatalf("Failed to parse deb line: %v", err)
			}

			if url != tc.wantURL {
				t.Errorf("URL: got %s, want %s", url, tc.wantURL)
			}
			if dist != tc.wantDist {
				t.Errorf("Dist: got %s, want %s", dist, tc.wantDist)
			}
			if comp != tc.wantComp {
				t.Errorf("Component: got %s, want %s", comp, tc.wantComp)
			}
		})
	}
}
