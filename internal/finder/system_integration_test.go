package finder

import (
	"testing"
)

// TestCodenameMapping is an integration test that verifies the current system's
// codename can be mapped successfully. This test will fail if running on a
// Debian system with an unknown codename, prompting us to update the mapping table.
func TestCodenameMapping(t *testing.T) {
	sysInfo, err := DetectSystem()
	if err != nil {
		t.Skipf("Could not detect system: %v", err)
	}

	// Try to get Ubuntu codename
	ubuntuCodename, err := sysInfo.GetUbuntuCodename()

	if sysInfo.OSName == "debian" {
		// For Debian systems, this should either succeed with a known mapping
		// or fail with an error prompting us to update the mapping
		if err != nil {
			t.Fatalf("Unknown Debian codename '%s' detected!\n"+
				"Please update the Debian→Ubuntu mapping table in system.go\n"+
				"Error: %v",
				sysInfo.Codename, err)
		}
		t.Logf("Debian %s maps to Ubuntu %s", sysInfo.Codename, ubuntuCodename)
	} else if sysInfo.OSName == "ubuntu" {
		// For Ubuntu systems, mapping should always succeed
		if err != nil {
			t.Fatalf("Unexpected error for Ubuntu system: %v", err)
		}
		t.Logf("Ubuntu codename: %s (LTS: %v)", ubuntuCodename, IsUbuntuLTS(ubuntuCodename))

		// Test LTS fallback for non-LTS systems
		if !IsUbuntuLTS(ubuntuCodename) {
			lts := GetNearestPastLTS(ubuntuCodename)
			if lts == "" {
				t.Errorf("Non-LTS Ubuntu %s has no LTS fallback defined", ubuntuCodename)
			} else {
				t.Logf("Non-LTS Ubuntu %s falls back to %s LTS", ubuntuCodename, lts)
			}
		}
	}
}

// TestLTSMapping verifies that all known non-LTS releases have valid LTS fallbacks
func TestLTSMapping(t *testing.T) {
	knownNonLTS := []string{
		"questing", "plucky", "oracular", "mantic", "lunar", "kinetic",
		"impish", "hirsute", "groovy", "eoan", "disco", "cosmic",
	}

	for _, codename := range knownNonLTS {
		lts := GetNearestPastLTS(codename)
		if lts == "" {
			t.Errorf("Non-LTS release %s has no LTS fallback", codename)
			continue
		}

		if !IsUbuntuLTS(lts) {
			t.Errorf("Fallback for %s (%s) is not marked as LTS", codename, lts)
		}
	}
}

// TestDebianMapping verifies that all known Debian releases have Ubuntu mappings
func TestDebianMapping(t *testing.T) {
	knownDebian := []struct {
		debian string
		ubuntu string
	}{
		{"trixie", "noble"},
		{"bookworm", "jammy"},
		{"bullseye", "focal"},
		{"buster", "bionic"},
		{"stretch", "xenial"},
	}

	for _, pair := range knownDebian {
		sysInfo := &SystemInfo{
			OSName:       "debian",
			Codename:     pair.debian,
			Architecture: "amd64",
		}

		ubuntuCodename, err := sysInfo.GetUbuntuCodename()
		if err != nil {
			t.Errorf("Debian %s mapping failed: %v", pair.debian, err)
			continue
		}

		if ubuntuCodename != pair.ubuntu {
			t.Errorf("Debian %s maps to %s, expected %s",
				pair.debian, ubuntuCodename, pair.ubuntu)
		}
	}
}

// TestAPIFallback verifies that the API-based mapping works for known Debian releases
func TestAPIFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping API test in short mode")
	}

	// Test with a known Debian release
	ubuntuCodename, err := GetUbuntuCodenameFromAPI("bookworm")
	if err != nil {
		t.Fatalf("API fallback failed for bookworm: %v", err)
	}

	// bookworm (June 2023) should map to jammy (April 2022) or noble (April 2024)
	if ubuntuCodename != "jammy" && ubuntuCodename != "noble" {
		t.Logf("Warning: bookworm mapped to %s (expected jammy or noble)", ubuntuCodename)
	} else {
		t.Logf("API correctly mapped Debian bookworm to Ubuntu %s", ubuntuCodename)
	}
}
