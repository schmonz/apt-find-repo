package finder

import (
	"testing"
)

func TestFilterSourcesForSystem(t *testing.T) {
	tests := []struct {
		name     string
		sources  []string
		sysInfo  *SystemInfo
		expected []string // Expected order (first is best match)
	}{
		{
			name: "exact-codename-match",
			sources: []string{
				"deb https://example.com focal main",
				"deb https://example.com jammy main",
				"deb https://example.com bookworm main",
			},
			sysInfo: &SystemInfo{
				OSName:       "ubuntu",
				Codename:     "jammy",
				Architecture: "amd64",
			},
			expected: []string{
				"deb https://example.com jammy main",
				"deb https://example.com focal main",
				// bookworm is filtered out (debian, not ubuntu)
			},
		},
		{
			name: "architecture-filtering",
			sources: []string{
				"deb [arch=arm64] https://example.com jammy main",
				"deb [arch=amd64] https://example.com jammy main",
				"deb https://example.com jammy main",
			},
			sysInfo: &SystemInfo{
				OSName:       "ubuntu",
				Codename:     "jammy",
				Architecture: "amd64",
			},
			expected: []string{
				"deb [arch=amd64] https://example.com jammy main",
				"deb https://example.com jammy main",
			},
		},
		{
			name: "dpkg-architecture-expansion",
			sources: []string{
				"deb [arch=$(dpkg --print-architecture)] https://example.com jammy main",
				"deb https://example.com jammy main",
			},
			sysInfo: &SystemInfo{
				OSName:       "ubuntu",
				Codename:     "jammy",
				Architecture: "amd64",
			},
			expected: []string{
				"deb [arch=$(dpkg --print-architecture)] https://example.com jammy main",
				"deb https://example.com jammy main",
			},
		},
		{
			name: "stable-vs-testing",
			sources: []string{
				"deb https://example.com testing main",
				"deb https://example.com stable main",
				"deb https://example.com unstable main",
			},
			sysInfo: &SystemInfo{
				OSName:       "debian",
				Codename:     "stable",
				Architecture: "amd64",
			},
			expected: []string{
				"deb https://example.com stable main",
				// testing and unstable have equal scores, order doesn't matter
			},
		},
		{
			name: "os-name-vs-other-os",
			sources: []string{
				"deb https://example.com bookworm main", // debian
				"deb https://example.com jammy main",    // ubuntu
				"deb https://example.com focal main",    // ubuntu
			},
			sysInfo: &SystemInfo{
				OSName:       "ubuntu",
				Codename:     "noble",
				Architecture: "amd64",
			},
			expected: []string{
				"deb https://example.com jammy main",
				"deb https://example.com focal main",
			},
		},
		{
			name: "generic-any-dist",
			sources: []string{
				"deb https://example.com any main",
				"deb https://example.com jammy main",
			},
			sysInfo: &SystemInfo{
				OSName:       "ubuntu",
				Codename:     "jammy",
				Architecture: "amd64",
			},
			expected: []string{
				"deb https://example.com jammy main",
				"deb https://example.com any main",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FilterSourcesForSystem(tc.sources, tc.sysInfo)

			// Check that we got at least the expected number of results
			if len(result) < len(tc.expected) {
				t.Errorf("Expected at least %d sources, got %d", len(tc.expected), len(result))
			}

			// Check order (only check as many as we expect)
			for i, expected := range tc.expected {
				if i >= len(result) {
					break
				}
				if result[i] != expected {
					t.Errorf("Position %d: expected %q, got %q", i, expected, result[i])
				}
			}
		})
	}
}

func TestScoreSource(t *testing.T) {
	sysInfo := &SystemInfo{
		OSName:       "ubuntu",
		Codename:     "jammy",
		Architecture: "amd64",
	}

	tests := []struct {
		name   string
		source string
		want   string // "positive", "negative", or "zero"
	}{
		{
			name:   "exact-match",
			source: "deb [arch=amd64] https://example.com jammy main",
			want:   "positive",
		},
		{
			name:   "wrong-architecture",
			source: "deb [arch=arm64] https://example.com jammy main",
			want:   "negative",
		},
		{
			name:   "wrong-os",
			source: "deb https://example.com bookworm main",
			want:   "negative",
		},
		{
			name:   "same-os-different-version",
			source: "deb https://example.com focal main",
			want:   "positive",
		},
		{
			name:   "invalid-source",
			source: "invalid line",
			want:   "negative",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			score := scoreSource(tc.source, sysInfo)
			switch tc.want {
			case "positive":
				if score <= 0 {
					t.Errorf("Expected positive score, got %d", score)
				}
			case "negative":
				if score >= 0 {
					t.Errorf("Expected negative score, got %d", score)
				}
			case "zero":
				if score != 0 {
					t.Errorf("Expected zero score, got %d", score)
				}
			}
		})
	}
}

func TestMatchKeysToSourcesWithSystem(t *testing.T) {
	tests := []struct {
		name      string
		gpgURLs   []string
		debLines  []string
		sysInfo   *SystemInfo
		wantFirst string // Expected first match debLine
		wantErr   bool
	}{
		{
			name: "prefer-matching-codename",
			gpgURLs: []string{
				"https://pkgs.example.com/gpg.key",
			},
			debLines: []string{
				"deb https://pkgs.example.com focal main",
				"deb https://pkgs.example.com jammy main",
			},
			sysInfo: &SystemInfo{
				OSName:       "ubuntu",
				Codename:     "jammy",
				Architecture: "amd64",
			},
			wantFirst: "deb https://pkgs.example.com jammy main",
			wantErr:   false,
		},
		{
			name: "filter-wrong-architecture",
			gpgURLs: []string{
				"https://pkgs.example.com/gpg.key",
			},
			debLines: []string{
				"deb [arch=arm64] https://pkgs.example.com jammy main",
				"deb [arch=amd64] https://pkgs.example.com jammy main",
			},
			sysInfo: &SystemInfo{
				OSName:       "ubuntu",
				Codename:     "jammy",
				Architecture: "amd64",
			},
			wantFirst: "deb [arch=amd64] https://pkgs.example.com jammy main",
			wantErr:   false,
		},
		{
			name: "no-system-info-uses-first",
			gpgURLs: []string{
				"https://pkgs.example.com/gpg.key",
			},
			debLines: []string{
				"deb https://pkgs.example.com focal main",
				"deb https://pkgs.example.com jammy main",
			},
			sysInfo:   nil,
			wantFirst: "deb https://pkgs.example.com focal main",
			wantErr:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matches, err := MatchKeysToSourcesWithSystem(tc.gpgURLs, tc.debLines, tc.sysInfo)

			if tc.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if len(matches) == 0 {
				t.Fatal("Expected at least one match, got none")
			}

			if matches[0].DebLine != tc.wantFirst {
				t.Errorf("Expected first match to be %q, got %q", tc.wantFirst, matches[0].DebLine)
			}
		})
	}
}
