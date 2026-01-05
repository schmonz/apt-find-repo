package finder

import (
	"bytes"
	"os"
	"testing"
)

type KeyTestCase struct {
	name       string
	inputFile  string
	wantFormat string // "armored", "binary", "dearmored"
	wantError  bool
}

var keyTestCases = []KeyTestCase{
	{
		name:       "armored-with-headers",
		inputFile:  "../../testdata/keys/armored-full.asc",
		wantFormat: "armored",
	},
	{
		name:       "armored-no-headers",
		inputFile:  "../../testdata/keys/armored-stripped.asc",
		wantFormat: "armored",
	},
	{
		name:       "binary-gpg",
		inputFile:  "../../testdata/keys/binary.gpg",
		wantFormat: "binary",
	},
	{
		name:       "dearmored-output",
		inputFile:  "../../testdata/keys/dearmored.gpg",
		wantFormat: "binary", // dearmored and binary are the same
	},
	{
		name:       "html-wrapped",
		inputFile:  "../../testdata/keys/html-wrapped.txt",
		wantFormat: "armored",
	},
	{
		name:      "not-a-key",
		inputFile: "../../testdata/keys/garbage.txt",
		wantError: true,
	},
}

func TestDetectKeyFormat(t *testing.T) {
	for _, tc := range keyTestCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile(tc.inputFile)
			if err != nil {
				t.Skipf("Test file not found: %s", tc.inputFile)
			}

			format, err := detectKeyFormat(data)
			if tc.wantError {
				if err == nil {
					t.Error("Expected error, got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if format != tc.wantFormat {
				t.Errorf("Format mismatch. Got %s, want %s", format, tc.wantFormat)
			}
		})
	}
}

func TestNormalizeKey(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		wantType string // what apt expects: "armored" or "binary"
	}{
		{
			name:     "armored-stays-armored",
			input:    []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----\n\nmQINBF...\n-----END PGP PUBLIC KEY BLOCK-----"),
			wantType: "armored",
		},
		{
			name:     "binary-stays-binary",
			input:    []byte{0x99, 0x01, 0x0d, 0x04}, // PGP binary marker
			wantType: "binary",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := NormalizeKey(tc.input)
			if err != nil {
				t.Fatalf("NormalizeKey failed: %v", err)
			}

			format, _ := detectKeyFormat(result)
			if format != tc.wantType {
				t.Errorf("Wrong output format. Got %s, want %s", format, tc.wantType)
			}
		})
	}
}

func TestExtractArmoredKey(t *testing.T) {
	// Key buried in HTML/text
	htmlWithKey := `
	<html><body>
	Some text here
	-----BEGIN PGP PUBLIC KEY BLOCK-----
	
	mQINBFzZ...
	-----END PGP PUBLIC KEY BLOCK-----
	More text
	</body></html>
	`

	key := extractArmoredKey([]byte(htmlWithKey))
	if key == nil {
		t.Fatal("Failed to extract key from HTML")
	}

	if !bytes.Contains(key, []byte("BEGIN PGP PUBLIC KEY BLOCK")) {
		t.Error("Extracted data doesn't look like a key")
	}
}

func TestKeyFetchAndNormalize(t *testing.T) {
	// Integration test - needs network
	if testing.Short() {
		t.Skip("Skipping network test")
	}

	testURLs := []struct {
		url      string
		wantType string
	}{
		{
			url:      "https://mirror.mwt.me/zoom/gpgkey",
			wantType: "armored",
		},
		{
			url:      "https://pkgs.tailscale.com/stable/ubuntu/jammy.noarmor.gpg",
			wantType: "binary",
		},
	}

	for _, tc := range testURLs {
		t.Run(tc.url, func(t *testing.T) {
			key, err := FetchKey(tc.url)
			if err != nil {
				t.Fatalf("Failed to fetch key: %v", err)
			}

			normalized, err := NormalizeKey(key)
			if err != nil {
				t.Fatalf("Failed to normalize: %v", err)
			}

			format, _ := detectKeyFormat(normalized)
			if format != tc.wantType {
				t.Errorf("Expected %s, got %s", tc.wantType, format)
			}
		})
	}
}
