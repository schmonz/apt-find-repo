# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`apt-find-repo` is a Go CLI tool that automatically discovers and adds third-party APT repositories from web pages. It scrapes web pages to find GPG keys and deb source lines, matches them together, fetches package lists, and optionally configures the repository on a Debian-based system.

## Build and Test Commands

Requires Go 1.21 or later.

```bash
# Build the binary
make build
# or: go build -o apt-find-repo

# Run the tool (list packages from a repo)
./apt-find-repo https://github.com/user/repo-name

# Run with verbose output (helpful for debugging)
./apt-find-repo -v https://github.com/user/repo-name

# Add a repository to the system (requires root)
sudo ./apt-find-repo https://github.com/user/repo-name add

# Run all tests
make test
# or: go test -v ./...

# Run specific tests
go test -v -run TestParseRepoPage
go test -v -run TestNormalizeKey
go test -v -run TestParsePackagesFile
go test -v -run TestMatchKeysToSources

# Clean build artifacts
make clean

# Install to system (requires root)
sudo make install

# Build Debian package
make deb
```

## Code Architecture

### Main Flow (main.go)

The program follows this pipeline:

1. **Fetch**: Downloads the web page from the provided URL
2. **Parse**: Extracts GPG key URLs and deb source lines using `parseRepoPage()`
3. **Match**: Pairs keys with sources by domain using `matchKeysToSources()`
4. **Execute**: Either lists packages or adds the repository

Two modes of operation:
- **List mode** (default): Fetches and displays available packages from the repository
- **Add mode** (`add` argument): Installs the repository to the system (requires root)

### Module Breakdown

**main.go**
- Entry point and orchestration
- `parseRepoPage()`: Scrapes HTML to find GPG keys and deb lines
- `findGPGKeys()`: Extracts URLs ending in .gpg, .asc, .key
- `findDebLines()`: Extracts deb source lines from code blocks and text
- `extractRepoName()`: Derives repository name from URL (e.g., GitHub repo name)

**validation.go**
- System validation and configuration generation
- `matchKeysToSources()`: Domain-based matching algorithm to pair keys with sources
- `generateKeyPath()`: Creates filesystem path for keys in /etc/apt/keyrings/
- `generateSourcesEntry()`: Converts deb lines to modern deb822 format (.sources files)
- `checkPrivileges()`, `checkDebianSystem()`, `checkAptDirectories()`: Pre-flight checks
- `checkConflicts()`: Prevents overwriting existing repository configurations

**packages.go**
- Package list fetching and parsing
- `parseDebLine()`: Parses deb source lines to extract URL, distribution, and component
- `fetchPackageList()`: Downloads Packages.gz or Packages files from the repository
- `parsePackagesFile()`: Extracts package names from Debian Packages file format

**key.go**
- GPG key handling
- `detectKeyFormat()`: Identifies armored vs binary PGP key formats
- `normalizeKey()`: Ensures keys are in apt-compatible format
- `fetchKey()`: Downloads GPG keys from URLs

### Key Design Decisions

- **Domain Matching**: Keys and sources are paired by matching their URL domains. This handles cases where a page describes multiple repositories or has multiple keys.
- **Modern APT Format**: Generates deb822 .sources files instead of legacy one-line-style sources.list entries.
- **Format Flexibility**: Handles GPG keys in multiple formats (armored .asc, binary .gpg, dearmored).
- **Fallback Fetching**: When finding deb lines, follows .list file URLs referenced in curl/wget commands.
- **Package Discovery**: Tries multiple architectures (amd64, all) and both compressed (.gz) and uncompressed Packages files.

### Test Structure

Tests use table-driven patterns with testdata files in testdata/ (organized into keys/, packages/, and webpages/ subdirectories):
- `packages_test.go`: Tests package file parsing with various Packages file formats
- `key_normalize_test.go`: Tests key format detection and normalization (armored, binary, dearmored)
- `repo_finder_test.go`: Tests HTML parsing and matching logic against real-world examples
- `validation_test.go`: Tests system checks and file generation logic

## Dependencies

- `github.com/PuerkitoBio/goquery`: HTML parsing and DOM traversal
- Standard library only otherwise

## Development Notes

- The tool is designed to work with free-form web pages (like GitHub READMEs) that describe how to add repositories
- Regex patterns in `findGPGKeys()` and `findDebLines()` are intentionally permissive to handle varied documentation styles
- The code prioritizes finding configuration in `<code>` and `<pre>` blocks over plain text for accuracy
