# Plan: Incremental Rewrite of apt-find-repo from Go to C

## Overview

Rewrite `apt-find-repo` from Go to C, incrementally, keeping tests passing
at every step. The C implementation will use CMake, libcurl, and other
shared libraries. The strategy is to build the C replacement module by
module, deleting each Go module as soon as its C replacement is proven.

## Guiding Principles

- **Tests pass at every step.** The existing test data (`testdata/`) and test
  expectations are the contract. We port tests to C (using `libcheck`) as we
  port the corresponding module, and keep them green.
- **Delete Go as we go.** After each module is ported and its C tests pass,
  we delete the corresponding Go source and test files. The Go codebase
  shrinks at every step — we never carry both implementations of the same
  module.
- **One module at a time.** Each step replaces one self-contained piece.
- **Shared test data.** The `testdata/` directory (webpages, keys, packages)
  is shared between Go and C test suites throughout the migration.
- **No Big Bang.** We don't try to wire everything together until each piece
  works in isolation.

## Go Intra-Package Dependencies (deletion constraints)

All Go source lives in `internal/finder/` (one package). Deleting a `.go`
file is safe only if no remaining `.go` file in the package references its
symbols.

```
key.go          → (nothing else references it)         — can delete immediately
packages.go     → used by validation.go (parseDebLine) — delete with or after validation
parser.go       → depends on system.go                 — can delete once its tests are gone
validation.go   → depends on system.go + packages.go   — delete frees packages.go too
system.go       → depended on by parser.go, validation  — must be last internal module deleted
```

## Steps

### Step 0: Refresh cached test data and verify Go tests pass

- Run `scripts/setup_tests.sh` to re-fetch all 44 cached webpages from
  their upstream URLs.
- Run `make test` and note any failures caused by upstream page changes.
- Update test expectations in `repo_finder_test.go` as needed to match
  current upstream content.
- Commit the refreshed test data and any expectation fixes.
- **Gate:** `make test` passes cleanly.

### Step 1: Set up C project skeleton with CMake

- Create `src/` for C source, `tests/` for C tests.
- Create `CMakeLists.txt` at the project root for the C build.
- Add a build dependency on `libcheck` (for C unit tests), `libcurl` (for
  HTTP), and `libgumbo` or `lexbor` (for HTML parsing — evaluate which is
  simpler; `libgumbo` is more widely packaged).
- Add a minimal `main.c` that prints usage and exits.
- Add a minimal C test file that passes.
- Update `Makefile` to offer both `make build-go` and `make build-c` (and
  similarly for test), keeping Go as the default for now.
- **Gate:** `cmake --build build && ctest --test-dir build` passes the
  trivial test. Go tests still pass.

### Step 2: Port `packages.go` → `packages.c`

This is the most self-contained module: parse a Debian `Packages` file to
extract package names, and parse a deb source line into URL/dist/component.

- Implement `parse_deb_line()` and `parse_packages_file()` in C.
- Port the 7 test cases from `packages_test.go` to C (reading from the same
  `testdata/packages/` files).
- Implement `fetch_package_list()` using libcurl (fetches Packages.gz,
  gunzips, parses).
- **Delete Go:** `packages_test.go` only (can't delete `packages.go` yet —
  `validation.go` still calls `parseDebLine`).
- **Gate:** C package-parsing tests pass. Remaining Go tests still pass.

### Step 3: Port `key.go` → `key.c`

GPG key format detection and normalization.

- Implement `detect_key_format()` and `normalize_key()` in C.
- Port the 9 test cases from `key_normalize_test.go` (reading from
  `testdata/keys/`).
- Implement `fetch_key()` using libcurl.
- **Delete Go:** `key.go` and `key_normalize_test.go` (nothing else in the
  package references key functions).
- **Gate:** C key tests pass. Remaining Go tests still pass.

### Step 4: Port `parser.go` → `parser.c`

HTML parsing to extract GPG URLs and deb lines. This is the largest and most
complex module.

- Implement `find_gpg_keys()` and `find_deb_lines()` in C, using libgumbo
  (or lexbor) for HTML DOM traversal, and POSIX regex or PCRE2 for pattern
  matching.
- Implement `is_valid_gpg_url()` filter.
- Implement `find_ppas()` for PPA extraction.
- Implement `dedup()` utility.
- Port the 45 test cases from `repo_finder_test.go` (reading from
  `testdata/webpages/`).
- Port the `TestFindDebLines` synthetic HTML tests.
- Port the `parser_test.go` GPG URL validation tests.
- **Delete Go:** `parser.go`, `parser_test.go`, and `repo_finder_test.go`
  (nothing remaining in the package references parser functions; `system.go`
  stays since `validation.go` still needs it).
- **Gate:** All C parser tests pass against the same test data. Remaining Go
  tests still pass (`validation_test.go`, `system_test.go`,
  `system_integration_test.go`).

### Step 5: Port `validation.go` → `validation.c`

Key-to-source matching, source scoring, and file path generation.

- Implement `match_keys_to_sources()` with domain-based matching.
- Implement `score_source()` and `filter_sources_for_system()`.
- Implement `generate_key_path()` and `generate_sources_entry()` (deb822
  format).
- Implement `check_privileges()`, `check_debian_system()`,
  `check_apt_directories()`, `check_conflicts()`.
- Port the test cases from `validation_test.go`.
- **Delete Go:** `validation.go`, `validation_test.go`, and now also
  `packages.go` (its last consumer within the package is gone).
- **Gate:** C validation tests pass. Remaining Go tests still pass
  (`system_test.go`, `system_integration_test.go`).

### Step 6: Port `system.go` → `system.c`

System detection: OS, codename, architecture, Debian→Ubuntu mapping.

- Implement `detect_system()` (reads `/etc/os-release`, calls `dpkg
  --print-architecture`).
- Implement `get_ubuntu_codename()` with static Debian→Ubuntu mapping.
- Implement `is_ubuntu_lts()` and `get_nearest_past_lts()`.
- Implement `get_ubuntu_codename_from_api()` API fallback using libcurl +
  a JSON parser (cJSON or json-c).
- Port the test cases from `system_test.go` and
  `system_integration_test.go`.
- **Delete Go:** `system.go`, `system_test.go`,
  `system_integration_test.go`. The `internal/finder/` directory is now
  empty — delete it and `internal/`.
- **Gate:** C system tests pass. No Go tests remain in `internal/`.

### Step 7: Port `main.go` → `main.c`

Wire everything together into the CLI entry point.

- Implement argument parsing (getopt or manual).
- Implement `search_for_repo()` (calls `ddgr`, parses JSON output with
  cJSON/json-c).
- Implement `find_install_scripts()` and `parse_install_script()`.
- Implement `validate_package_glob()` (glob/fnmatch matching).
- Implement `etckeeper_commit()`.
- Implement the main pipeline: fetch page → parse → match → fetch packages
  → validate → install.
- Test the full C binary against the same manual smoke tests used with Go.
- **Delete Go:** `cmd/apt-find-repo/main.go`, `cmd/` directory, `go.mod`,
  `go.sum`.
- **Gate:** The C binary produces equivalent output to the Go binary for the
  same inputs. `make test` passes with only C. No Go source remains.

### Step 8: Clean up

- Update `Makefile` so `make build` and `make test` target the C
  implementation (remove Go-era targets).
- Update `CLAUDE.md` and `README.md` to document the C build.
- Update `debian/` packaging if present.
- Remove `.go`-related entries from `.gitignore`.
- **Gate:** `make test` passes. Manual smoke tests pass. Clean repo.

## Dependency Summary

| Library | Purpose | Debian package |
|---------|---------|----------------|
| libcurl | HTTP fetches | `libcurl4-openssl-dev` |
| libgumbo | HTML parsing | `libgumbo-dev` |
| libcheck | C unit testing | `check` |
| cJSON or json-c | JSON parsing (ddgr output, APIs) | `libcjson-dev` or `libjson-c-dev` |
| zlib | gzip decompression (Packages.gz) | `zlib1g-dev` |
| PCRE2 (optional) | Regex if POSIX ERE insufficient | `libpcre2-dev` |

## Risk Notes

- **HTML parsing fidelity**: The Go implementation uses `goquery` (CSS
  selectors over parsed HTML). `libgumbo` provides a parsed DOM but no CSS
  selector engine — we'll traverse the tree directly looking for `<code>` and
  `<pre>` elements. This is actually closer to what the Go code does (it
  finds elements then regex-matches their text content).
- **Regex complexity**: The Go code uses Go's RE2-style regex. C's POSIX ERE
  should handle most patterns; PCRE2 is a fallback if needed.
- **Memory management**: Each module will own its allocations and provide
  cleanup functions. Tests will use valgrind/ASan to check for leaks.
