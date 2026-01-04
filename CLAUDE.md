# CLAUDE.md - AI Assistant Guide for apt-find-repo

## Project Overview

**apt-find-repo** is a Go-based command-line tool that simplifies adding third-party APT repositories to Debian-based systems. It automatically discovers and configures repositories from web pages by extracting GPG keys and repository sources.

**Primary Use Case**: Users provide a URL to a repository webpage, and the tool either:
1. Lists available packages from that repository (default mode)
2. Configures the repository on the system (`add` mode)

**Example Usage**:
```bash
# Preview packages
apt-find-repo https://github.com/JonasGroeger/jetbrains-ppa

# Add repository (requires root)
sudo apt-find-repo https://github.com/JonasGroeger/jetbrains-ppa add
```

## Repository Structure

```
apt-find-repo/
├── main.go                    # Entry point, CLI handling, main workflow
├── validation.go              # Repository matching, path generation, system validation
├── packages.go                # Package list fetching and parsing
├── key.go                     # GPG key handling and normalization
├── *_test.go                  # Unit tests for each module
├── testdata/                  # Test fixtures
│   ├── webpages/             # HTML snapshots of real repository pages
│   ├── keys/                 # Various GPG key format examples
│   └── packages/             # Package list samples
├── setup_*_tests.sh          # Test data setup scripts
├── Makefile                  # Build, test, install targets
├── go.mod                    # Go module definition
└── apt-find-repo.1           # Man page
```

## Core Files and Responsibilities

### main.go (7617 bytes)
**Primary Responsibilities**:
- CLI argument parsing (URL, `-v` flag, `add` command)
- HTTP fetching of repository webpages
- Orchestration of the complete workflow
- User output and error handling

**Key Functions**:
- `main()`: Entry point, orchestrates entire flow
- `parseRepoPage()`: Extracts GPG URLs and deb lines from HTML
- `findGPGKeys()`: Pattern matching for GPG key URLs (.gpg, .asc, .key, /gpg)
- `findDebLines()`: Regex extraction of `deb` source lines from code blocks
- `extractRepoName()`: Derives repository name from URL (GitHub-aware)
- `dedup()`: Removes duplicate findings

**HTML Parsing Strategy**:
1. Prioritizes code blocks (`<code>`, `<pre>`) as most reliable
2. Falls back to full-text search if needed
3. Follows `.list` file URLs if found in curl/wget commands

**Lines**: main.go:1-301

### validation.go (5377 bytes)
**Primary Responsibilities**:
- Matching GPG keys to deb sources by domain
- Generating filesystem paths for keys and sources files
- System validation (root privileges, Debian detection, directory checks)
- Conflict detection (existing keys/sources)

**Key Functions**:
- `matchKeysToSources()`: Pairs keys with sources using domain matching logic
- `generateKeyPath()`: Creates `/etc/apt/keyrings/<name>.(gpg|asc)` paths
- `generateSourcesEntry()`: Converts deb lines to deb822 format
- `checkPrivileges()`: Verifies root access (EUID = 0)
- `checkDebianSystem()`: Confirms dpkg and apt presence
- `checkConflicts()`: Prevents overwriting existing files

**Path Generation Convention**:
- Keys: `/etc/apt/keyrings/<sanitized-repo-name>.(gpg|asc)`
- Sources: `/etc/apt/sources.list.d/<sanitized-repo-name>.sources`
- Sanitization: lowercase, non-alphanumeric to `-`, collapse multiple dashes

**Lines**: validation.go:1-217

### packages.go (2851 bytes)
**Primary Responsibilities**:
- Parsing deb source lines into components (URL, dist, component)
- Fetching Packages files from repositories
- Extracting package names

**Key Functions**:
- `parseDebLine()`: Extracts URL, distribution, and component from deb line
- `fetchPackageList()`: Tries multiple architectures (amd64, all) and formats (.gz, plain)
- `fetchAndParsePackages()`: Downloads and decompresses Packages files
- `parsePackagesFile()`: Scans for `Package:` fields

**Package List URL Pattern**:
```
{repo-url}/dists/{dist}/{component}/binary-{arch}/Packages[.gz]
```

**Lines**: packages.go:1-124

### key.go (2376 bytes)
**Primary Responsibilities**:
- Detecting GPG key format (armored, binary, dearmored)
- Normalizing keys for APT compatibility
- Fetching keys from URLs

**Key Functions**:
- `detectKeyFormat()`: Identifies key type by examining bytes
- `normalizeKey()`: Ensures key is usable by APT
- `extractArmoredKey()`: Extracts PGP block from surrounding text
- `fetchKey()`: Downloads key data via HTTP

**Supported Formats**:
- Armored: ASCII-armored PGP blocks (BEGIN/END markers)
- Binary: Native GPG binary format (starts with 0x99 or 0x80-0xBF)
- Dearmored: Binary without markers (detected by entropy)

**Lines**: key.go:1-104

## Testing Strategy

### Test Organization
All tests follow Go's standard `*_test.go` naming convention and live alongside their source files.

### Test Files
- `repo_finder_test.go`: Web scraping and parsing tests
- `validation_test.go`: Matching, path generation, system checks
- `packages_test.go`: Package list parsing (requires setup script)
- `key_normalize_test.go`: Key format detection (requires setup script)

### Test Data Management
- **testdata/webpages/**: Real HTML snapshots (jetbrains, tailscale, zoom)
- **testdata/keys/**: Various key formats for format detection tests
- **testdata/packages/**: Sample Packages files

**Setup Scripts**:
- `setup_tests.sh`: Master setup (calls other setup scripts)
- `setup_key_tests.sh`: Generates test keys in various formats
- `setup_packages_tests.sh`: Creates sample package files

**Running Tests**:
```bash
# Setup test data first
./setup_tests.sh

# Run all tests
make test
# or
go test -v ./...
```

### Test Patterns
- Table-driven tests with `tests := []struct{...}` pattern
- TestMain() ensures testdata directory exists
- Tests are environment-aware (e.g., won't fail on non-Debian systems)

**Example Test Structure** (validation_test.go:7-101):
```go
tests := []struct {
    name        string
    gpgURLs     []string
    debLines    []string
    wantMatches []Match
    wantErr     bool
}{...}
```

## Development Workflow

### Building
```bash
make build           # Produces ./apt-find-repo binary
```

### Testing
```bash
make test            # Runs go test -v ./...
```

### Installing
```bash
make install         # Installs to $(DESTDIR)/usr/bin/
                     # Also installs man page to /usr/share/man/man1/
```

### Cleaning
```bash
make clean           # Removes binary and cache directories
```

### Debian Packaging
```bash
make deb             # Creates .deb package (requires dpkg-buildpackage)
```

### Code Formatting
The project uses standard Go formatting:
```bash
go fmt ./...
```

**Recent commit shows formatting enforcement**: commit `26a2271 go fmt ./...`

## Dependencies

**Runtime Dependencies** (go.mod):
- Go 1.21+
- `github.com/PuerkitoBio/goquery v1.8.1` - HTML parsing

**Transitive Dependencies**:
- `github.com/andybalholm/cascadia v1.3.1` - CSS selectors
- `golang.org/x/net v0.7.0` - Network utilities

**No external runtime dependencies** - single statically-linked binary

## Code Conventions

### Error Handling
- All errors returned to user via `fmt.Fprintf(os.Stderr, ...)`
- Exit codes: `os.Exit(1)` on all errors
- Verbose mode (`-v`) provides additional diagnostic output

### Verbose Logging Pattern
```go
if verbose {
    fmt.Fprintf(os.Stderr, "Fetching %s...\n", url)
}
```

### Regular Expressions
Heavy use of regex for parsing:
- GPG key URLs: `https?://[^\s<>"']+\.(gpg|asc|key)`
- Deb lines: `deb\s+(?:\[[^\]]+\]\s+)?https?://[^\s]+(?:\s+[a-zA-Z0-9][a-zA-Z0-9._-]*)+`
- Compile once, use repeatedly

### Domain-Based Matching
The tool assumes **one key per domain** for matching. Multiple keys from the same domain for the same source is treated as ambiguous and errors.

### File Operations
- Uses `os.WriteFile()` for atomic writes
- Creates directories with `os.MkdirAll(dir, 0755)`
- Checks existence before writing to prevent conflicts

## Key Design Decisions

### Modern APT Format (deb822)
The tool generates modern deb822-style `.sources` files instead of legacy one-line format:
```
Types: deb
URIs: https://example.com/repo
Suites: stable
Components: main
Signed-By: /etc/apt/keyrings/example.gpg
```

### Keyring Location
Keys stored in `/etc/apt/keyrings/` (modern APT standard) rather than deprecated `/etc/apt/trusted.gpg.d/`

### Conflict Prevention
The tool **refuses to overwrite** existing keys or sources. This prevents accidental repository configuration corruption.

### Multi-Architecture Support
When fetching package lists, tries: `amd64`, then `all` architectures

### Progressive Fallback
For package files, tries `.gz` compressed first (smaller), falls back to uncompressed

## Known Limitations

From TODO.md and commit history:

### Not Yet Supported
- ❌ Zoom official repository
- ❌ Tailscale official repository (partially detected but not working)
- ❌ Many other repository types

**See testdata/webpages/** for examples:
- ✅ JetBrains PPA (working): jetbrains-unofficial.html
- ⚠️ Tailscale (detected, not working): tailscale-official.html
- ⚠️ Zoom (detected, not working): zoom-unofficial.html

### Future Plans (TODO.md)
1. Handle Zoom and Tailscale repos
2. Add support for more repository examples
3. Organize code into standard Go layout: `/cmd`, `/internal`, `/scripts`, `/build`, `/test`, `/docs`
4. Add Go Report Card badge
5. Create Debian packages via OpenBuildService
6. Publish Homebrew tap for macOS support

## AI Assistant Guidelines

### When Making Changes

1. **Always Read Before Editing**: Use Read tool on files before modifying them
2. **Run Tests**: Execute `./setup_tests.sh && make test` after changes
3. **Format Code**: Run `go fmt ./...` before committing
4. **Update Tests**: Add test cases for new functionality
5. **Check Examples**: Ensure existing test cases (JetBrains, Tailscale, Zoom) still pass

### Common Tasks

#### Adding Support for a New Repository Type
1. Add HTML snapshot to `testdata/webpages/<name>.html`
2. Add test case to `repo_finder_test.go` testCases array
3. Update parsing logic in `findGPGKeys()` or `findDebLines()` if needed
4. Run tests to verify

#### Debugging Repository Detection
1. Use `-v` flag: `apt-find-repo -v <url>`
2. Check what GPG URLs and deb lines were found
3. Add test case with actual HTML to testdata/webpages/
4. Run specific test: `go test -v -run TestParseRepoPage/<name>`

#### Testing Package List Fetching
1. Ensure test data exists: `./setup_packages_tests.sh`
2. Run: `go test -v -run TestFetchPackageList`

#### Testing Key Normalization
1. Ensure test keys exist: `./setup_key_tests.sh`
2. Run: `go test -v -run TestNormalizeKey`

### Code Modification Best Practices

1. **Maintain Table-Driven Tests**: When adding features, extend test tables rather than creating separate test functions
2. **Preserve Error Messages**: User-facing errors should be clear and actionable
3. **Keep Single Binary**: Don't add dependencies that require external tools at runtime
4. **Validate Debian Compatibility**: Test on Debian/Ubuntu systems when possible
5. **Consider Permissions**: Remember `add` mode requires root, package listing doesn't

### Understanding the Flow

**List Mode** (no `add` argument):
```
URL → HTTP GET → HTML Parse → Extract GPG/Deb → Match → Fetch Packages → Print
```

**Add Mode** (with `add` argument):
```
URL → HTTP GET → HTML Parse → Extract GPG/Deb → Match
  → Validate System → Check Conflicts → Fetch Key → Normalize Key
  → Generate Paths → Write Key → Write Sources
```

### Important File Locations

When the tool runs in `add` mode as root:
- **Keys written to**: `/etc/apt/keyrings/<repo-name>.(gpg|asc)`
- **Sources written to**: `/etc/apt/sources.list.d/<repo-name>.sources`

### Regex Patterns to Understand

**GPG Key Detection** (main.go:205-209):
```go
`https?://[^\s<>"']+\.(?:gpg|asc|key)`
`https?://[^\s<>"']+/gpg`
`https?://[^\s<>"']+\.noarmor\.gpg`
```

**Deb Line Detection** (main.go:228):
```go
`deb\s+(?:\[[^\]]+\]\s+)?https?://[^\s]+(?:\s+[a-zA-Z0-9][a-zA-Z0-9._-]*)+`
```

**Repository Name from GitHub URL** (main.go:173-174):
```go
`github\.com/[^/]+/([^/]+)`
```

### Testing New Regex Patterns

When modifying parsing logic, add test cases to `TestFindDebLines` in repo_finder_test.go following the existing pattern.

## Git Workflow

### Branch Strategy
- Currently working on: `claude/add-claude-documentation-QgAAH`
- Default development appears to be direct to main branch

### Commit Message Style
Looking at recent commits:
- Imperative mood: "go fmt ./..." not "formatted code"
- Descriptive: "Detect JetBrains repo. No Zoom or Tailscale yet."
- Keep commits focused and atomic

### Before Committing
```bash
go fmt ./...                  # Format code
./setup_tests.sh && make test # Run tests
make build                     # Ensure it compiles
```

## Architecture Notes

### No External State
The tool is **stateless** - all operations based on:
1. Input URL
2. Fetched webpage content
3. Current filesystem state (in `add` mode)

### No Configuration File
No `~/.config/apt-find-repo` or similar. All behavior controlled by command-line arguments.

### Idempotency
`add` mode is **not idempotent** - it errors if files already exist rather than updating them. This is intentional to prevent accidental overwrites.

### Single Responsibility per File
- main.go: CLI and orchestration
- validation.go: Validation and path generation
- packages.go: Package list operations
- key.go: Key handling

This separation makes it easy to test each component independently.

## Quick Reference

### Running the Tool
```bash
# Development build
go build -o apt-find-repo

# List packages from a repository
./apt-find-repo https://github.com/JonasGroeger/jetbrains-ppa

# List with verbose output
./apt-find-repo -v https://github.com/JonasGroeger/jetbrains-ppa

# Add repository (requires root)
sudo ./apt-find-repo https://github.com/JonasGroeger/jetbrains-ppa add
```

### Testing Specific Components
```bash
go test -v -run TestParseRepoPage        # Web parsing
go test -v -run TestMatchKeysToSources   # Matching logic
go test -v -run TestGenerateKeyPath      # Path generation
go test -v -run TestNormalizeKey         # Key handling
```

### Adding Test Data
```bash
# Save webpage HTML
curl -L <url> > testdata/webpages/name.html

# Add to testCases in repo_finder_test.go
# Run test
go test -v -run TestParseRepoPage/name
```

## Summary

This is a **focused, single-purpose tool** that does one thing well: automatically configuring third-party APT repositories. The code is straightforward Go with minimal dependencies, comprehensive tests, and clear separation of concerns. When extending it, maintain these qualities and ensure new repository types are added as test cases first.
