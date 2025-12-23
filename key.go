package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
)

// detectKeyFormat identifies the format of PGP key data
func detectKeyFormat(data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("empty data")
	}

	// Check for PGP binary format (packet tag byte)
	// Binary keys start with 0x99 (old format) or 0x80-0xBF (new format)
	if data[0] == 0x99 || (data[0] >= 0x80 && data[0] <= 0xBF) {
		return "binary", nil
	}

	// Check for armored format
	if bytes.Contains(data, []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----")) {
		return "armored", nil
	}

	// Check if it looks like dearmored output (high entropy binary)
	// Heuristic: if most bytes are non-printable, probably binary
	nonPrintable := 0
	sample := data
	if len(data) > 100 {
		sample = data[:100]
	}

	for _, b := range sample {
		if b < 32 || b > 126 {
			nonPrintable++
		}
	}

	if float64(nonPrintable)/float64(len(sample)) > 0.3 {
		return "dearmored", nil
	}

	return "", fmt.Errorf("unrecognized key format")
}

// normalizeKey ensures key is in a format apt can use
// Returns the key data unchanged - we just validate it's usable
func normalizeKey(data []byte) ([]byte, error) {
	format, err := detectKeyFormat(data)
	if err != nil {
		return nil, err
	}

	switch format {
	case "armored":
		// Extract just the key block if embedded in other text
		extracted := extractArmoredKey(data)
		if extracted == nil {
			return nil, fmt.Errorf("failed to extract armored key")
		}
		return extracted, nil

	case "binary", "dearmored":
		// Binary formats are fine as-is
		return data, nil

	default:
		return nil, fmt.Errorf("unknown format: %s", format)
	}
}

// extractArmoredKey pulls out the PGP key block from surrounding text
func extractArmoredKey(data []byte) []byte {
	re := regexp.MustCompile(`(?s)-----BEGIN PGP PUBLIC KEY BLOCK-----.*?-----END PGP PUBLIC KEY BLOCK-----`)
	match := re.Find(data)
	if match == nil {
		return nil
	}
	return match
}

// fetchKey downloads a key from a URL
func fetchKey(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}

	return data, nil
}
