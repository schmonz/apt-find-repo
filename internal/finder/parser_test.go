package finder

import (
	"testing"
)

func TestIsValidGPGURL(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		valid bool
	}{
		{
			name:  "valid-gpg-url",
			url:   "https://packages.example.com/keys/repo.gpg",
			valid: true,
		},
		{
			name:  "valid-asc-url",
			url:   "https://packages.example.com/keys/repo.asc",
			valid: true,
		},
		{
			name:  "valid-key-url",
			url:   "https://packages.example.com/keys/repo.key",
			valid: true,
		},
		{
			name:  "valid-gpg-path",
			url:   "https://packages.example.com/gpg",
			valid: true,
		},
		{
			name:  "bash-brace-expansion",
			url:   "https://dl.brave.com/install.sh{,.asc",
			valid: false,
		},
		{
			name:  "bash-brace-expansion-closing",
			url:   "https://example.com/key.gpg}",
			valid: false,
		},
		{
			name:  "multi-line-config",
			url:   "https://packages.microsoft.com/yumrepos/vscode\nenabled=1\nautorefresh=1",
			valid: false,
		},
		{
			name:  "config-with-equals",
			url:   "https://example.com/repo?key=value.gpg",
			valid: false,
		},
		{
			name:  "config-with-brackets",
			url:   "https://example.com/repo[arch=amd64].gpg",
			valid: false,
		},
		{
			name:  "install-script-signature",
			url:   "https://example.com/install.sh.asc",
			valid: false,
		},
		{
			name:  "install-script-in-path",
			url:   "https://example.com/install.sh-key.gpg",
			valid: false,
		},
		{
			name:  "carriage-return",
			url:   "https://example.com/key.gpg\r\nmalicious",
			valid: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isValidGPGURL(tc.url)
			if got != tc.valid {
				t.Errorf("isValidGPGURL(%q) = %v, want %v", tc.url, got, tc.valid)
			}
		})
	}
}
