package finder

import (
	"testing"
)

func TestMatchKeysToSources(t *testing.T) {
	tests := []struct {
		name        string
		gpgURLs     []string
		debLines    []string
		wantMatches []Match
		wantErr     bool
	}{
		{
			name:     "single-match",
			gpgURLs:  []string{"https://example.com/key.gpg"},
			debLines: []string{"deb https://example.com/repo stable main"},
			wantMatches: []Match{
				{
					GPGURL:  "https://example.com/key.gpg",
					DebLine: "deb https://example.com/repo stable main",
				},
			},
		},
		{
			name:     "same-domain-match",
			gpgURLs:  []string{"https://mirror.mwt.me/zoom/gpgkey"},
			debLines: []string{"deb https://mirror.mwt.me/zoom/deb any main"},
			wantMatches: []Match{
				{
					GPGURL:  "https://mirror.mwt.me/zoom/gpgkey",
					DebLine: "deb https://mirror.mwt.me/zoom/deb any main",
				},
			},
		},
		{
			name: "multiple-matches-by-domain",
			gpgURLs: []string{
				"https://repo-a.com/key.gpg",
				"https://repo-b.com/key.gpg",
			},
			debLines: []string{
				"deb https://repo-a.com/deb stable main",
				"deb https://repo-b.com/deb stable main",
			},
			wantMatches: []Match{
				{
					GPGURL:  "https://repo-a.com/key.gpg",
					DebLine: "deb https://repo-a.com/deb stable main",
				},
				{
					GPGURL:  "https://repo-b.com/key.gpg",
					DebLine: "deb https://repo-b.com/deb stable main",
				},
			},
		},
		{
			name:     "ambiguous-multiple-keys-one-source",
			gpgURLs:  []string{"https://a.com/key1.gpg", "https://a.com/key2.gpg"},
			debLines: []string{"deb https://a.com/repo stable main"},
			wantErr:  true,
		},
		{
			name:    "one-key-multiple-sources-same-domain",
			gpgURLs: []string{"https://mirror.mwt.me/zoom/gpgkey"},
			debLines: []string{
				"deb [arch=amd64 signed-by=/etc/apt/keyrings/mwt.asc by-hash=force] https://mirror.mwt.me/zoom/deb any main",
				"deb [arch=amd64 signed-by=/etc/apt/keyrings/mwt.asc by-hash=force] https://mirror.mwt.me/rstudio/deb/jammy jammy main",
			},
			wantMatches: []Match{
				{
					GPGURL:  "https://mirror.mwt.me/zoom/gpgkey",
					DebLine: "deb [arch=amd64 signed-by=/etc/apt/keyrings/mwt.asc by-hash=force] https://mirror.mwt.me/zoom/deb any main",
				},
				{
					GPGURL:  "https://mirror.mwt.me/zoom/gpgkey",
					DebLine: "deb [arch=amd64 signed-by=/etc/apt/keyrings/mwt.asc by-hash=force] https://mirror.mwt.me/rstudio/deb/jammy jammy main",
				},
			},
		},
		{
			name: "multiple-keys-and-sources-path-based-match",
			gpgURLs: []string{
				"https://pkgs.tailscale.com/stable/ubuntu/jammy.noarmor.gpg",
				"https://pkgs.tailscale.com/stable/ubuntu/focal.noarmor.gpg",
			},
			debLines: []string{
				"deb [signed-by=/usr/share/keyrings/tailscale-archive-keyring.gpg] https://pkgs.tailscale.com/stable/ubuntu jammy main",
				"deb [signed-by=/usr/share/keyrings/tailscale-archive-keyring.gpg] https://pkgs.tailscale.com/stable/ubuntu focal main",
			},
			wantMatches: []Match{
				{
					GPGURL:  "https://pkgs.tailscale.com/stable/ubuntu/focal.noarmor.gpg",
					DebLine: "deb [signed-by=/usr/share/keyrings/tailscale-archive-keyring.gpg] https://pkgs.tailscale.com/stable/ubuntu focal main",
				},
				{
					GPGURL:  "https://pkgs.tailscale.com/stable/ubuntu/jammy.noarmor.gpg",
					DebLine: "deb [signed-by=/usr/share/keyrings/tailscale-archive-keyring.gpg] https://pkgs.tailscale.com/stable/ubuntu jammy main",
				},
			},
		},
		{
			name:     "no-matches",
			gpgURLs:  []string{},
			debLines: []string{},
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matches, err := MatchKeysToSources(tc.gpgURLs, tc.debLines)

			if tc.wantErr {
				if err == nil {
					t.Error("Expected error, got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if len(matches) != len(tc.wantMatches) {
				t.Fatalf("Match count: got %d, want %d", len(matches), len(tc.wantMatches))
			}

			for i, match := range matches {
				if match.GPGURL != tc.wantMatches[i].GPGURL {
					t.Errorf("Match[%d] GPG: got %s, want %s", i, match.GPGURL, tc.wantMatches[i].GPGURL)
				}
				if match.DebLine != tc.wantMatches[i].DebLine {
					t.Errorf("Match[%d] Deb: got %s, want %s", i, match.DebLine, tc.wantMatches[i].DebLine)
				}
			}
		})
	}
}

func TestGenerateKeyPath(t *testing.T) {
	tests := []struct {
		name       string
		gpgURL     string
		repoName   string
		wantPath   string
		wantFormat string
	}{
		{
			name:       "from-url-armored",
			gpgURL:     "https://example.com/repo/key.asc",
			repoName:   "example-repo",
			wantPath:   "/etc/apt/keyrings/example-repo.asc",
			wantFormat: "armored",
		},
		{
			name:       "from-url-binary",
			gpgURL:     "https://example.com/key.gpg",
			repoName:   "example-repo",
			wantPath:   "/etc/apt/keyrings/example-repo.gpg",
			wantFormat: "binary",
		},
		{
			name:       "sanitize-name",
			gpgURL:     "https://example.com/key.gpg",
			repoName:   "My Repo (unofficial)",
			wantPath:   "/etc/apt/keyrings/my-repo-unofficial.gpg",
			wantFormat: "binary",
		},
		{
			name:       "package-glob-with-wildcard",
			gpgURL:     "https://example.com/key.gpg",
			repoName:   "tailscale*",
			wantPath:   "/etc/apt/keyrings/tailscale.gpg",
			wantFormat: "binary",
		},
		{
			name:       "package-glob-simple",
			gpgURL:     "https://example.com/key.asc",
			repoName:   "zoom-client",
			wantPath:   "/etc/apt/keyrings/zoom-client.asc",
			wantFormat: "armored",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path, format := GenerateKeyPath(tc.gpgURL, tc.repoName)

			if path != tc.wantPath {
				t.Errorf("Path: got %s, want %s", path, tc.wantPath)
			}
			if format != tc.wantFormat {
				t.Errorf("Format: got %s, want %s", format, tc.wantFormat)
			}
		})
	}
}

func TestGenerateSourcesEntry(t *testing.T) {
	tests := []struct {
		name         string
		debLine      string
		keyPath      string
		wantEntry    string
		wantFilename string
	}{
		{
			name:    "simple-deb822",
			debLine: "deb https://example.com/repo stable main",
			keyPath: "/etc/apt/keyrings/example.gpg",
			wantEntry: `Types: deb
URIs: https://example.com/repo
Suites: stable
Components: main
Signed-By: /etc/apt/keyrings/example.gpg`,
			wantFilename: "example.sources",
		},
		{
			name:    "with-arch-option",
			debLine: "deb [arch=amd64] https://example.com/repo stable main",
			keyPath: "/etc/apt/keyrings/example.gpg",
			wantEntry: `Types: deb
URIs: https://example.com/repo
Suites: stable
Components: main
Architectures: amd64
Signed-By: /etc/apt/keyrings/example.gpg`,
			wantFilename: "example.sources",
		},
		{
			name:    "multiple-components",
			debLine: "deb https://example.com/repo focal main universe multiverse",
			keyPath: "/etc/apt/keyrings/example.gpg",
			wantEntry: `Types: deb
URIs: https://example.com/repo
Suites: focal
Components: main universe multiverse
Signed-By: /etc/apt/keyrings/example.gpg`,
			wantFilename: "example.sources",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			entry, filename := GenerateSourcesEntry(tc.debLine, tc.keyPath)

			if entry != tc.wantEntry {
				t.Errorf("Entry:\ngot:\n%s\n\nwant:\n%s", entry, tc.wantEntry)
			}
			if filename != tc.wantFilename {
				t.Errorf("Filename: got %s, want %s", filename, tc.wantFilename)
			}
		})
	}
}

func TestCheckPrivileges(t *testing.T) {
	hasRoot := CheckPrivileges()
	// Just verify it returns without error
	// Actual value depends on test environment
	t.Logf("Running with root privileges: %v", hasRoot)
}

func TestCheckDebianSystem(t *testing.T) {
	isDebian := CheckDebianSystem()
	// Log result - actual value depends on test environment
	t.Logf("Running on Debian-based system: %v", isDebian)
}

func TestCheckAptDirectories(t *testing.T) {
	err := CheckAptDirectories()
	// Log result - actual value depends on test environment
	if err != nil {
		t.Logf("APT directories check failed (expected on non-Debian): %v", err)
	} else {
		t.Logf("APT directories check passed")
	}
}

func TestCheckConflicts(t *testing.T) {
	tests := []struct {
		name             string
		keyPath          string
		sourcesFilename  string
		setupFunc        func() // create test files
		cleanupFunc      func() // remove test files
		wantKeyExists    bool
		wantSourceExists bool
	}{
		{
			name:             "no-conflicts",
			keyPath:          "/tmp/test-repo-finder-key.gpg",
			sourcesFilename:  "test-repo-finder.list",
			wantKeyExists:    false,
			wantSourceExists: false,
		},
		// We'll skip actual file creation tests for now
		// since they need temp directories
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setupFunc != nil {
				tc.setupFunc()
				defer tc.cleanupFunc()
			}

			keyExists, sourceExists := CheckConflicts(tc.keyPath, tc.sourcesFilename)

			if keyExists != tc.wantKeyExists {
				t.Errorf("Key exists: got %v, want %v", keyExists, tc.wantKeyExists)
			}
			if sourceExists != tc.wantSourceExists {
				t.Errorf("Source exists: got %v, want %v", sourceExists, tc.wantSourceExists)
			}
		})
	}
}
