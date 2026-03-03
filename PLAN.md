# Plan: Incremental Rewrite of apt-find-repo from Go to C

## Overview

Rewrite `apt-find-repo` from Go to C, incrementally, keeping tests passing
at every step. The C implementation will use CMake, libcurl, and other
shared libraries. The strategy is to build the C replacement module by
module, deleting each Go module immediately by using cgo shims to bridge
the C library into the Go binary during the transition.

## Guiding Principles

- **Tests pass at every step.** The existing test data (`testdata/`) and test
  expectations are the contract. We port tests to C (using `libcheck`) as we
  port the corresponding module, and keep them green.
- **Delete Go as we go.** After each module is ported, we replace the Go
  source file with a thin cgo shim calling the C library, then delete the
  Go implementation. No module exists in both languages simultaneously.
- **One module at a time.** Each step replaces one self-contained piece.
- **Shared test data.** The `testdata/` directory (webpages, keys, packages)
  is shared between Go and C test suites throughout the migration.
- **No Big Bang.** We don't try to wire everything together until each piece
  works in isolation.
- **Reorganize at the end.** The C code's initial layout mirrors the Go
  modules for ease of translation. Once all Go is gone, we're free to
  reorganize the C source, headers, tests, and build however we like.

## The cgo Bridge Strategy

Go supports calling C code via `cgo`. At each porting step:

1. Build the new C module into `libaptfindrepo.a` (static library, grows
   each step).
2. Replace the Go source file (e.g. `packages.go`) with a small cgo shim
   (e.g. `packages_cgo.go`) that `// #cgo` links the C library and wraps
   each C function with the original Go function signature.
3. Delete the original Go implementation and its Go tests.
4. Remaining Go code that calls those functions keeps compiling — it now
   goes through cgo to C.
5. When the last caller of a shim is itself ported to C, delete the shim.

This means every Go source file is deleted at its own step. Nothing lingers.

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
- Configure CMake to produce `libaptfindrepo.a` (static library) and a
  `apt-find-repo` binary (initially a stub).
- Add build dependencies: `libcheck` (C unit tests), `libcurl` (HTTP),
  `libgumbo` or `lexbor` (HTML parsing — evaluate which is simpler;
  `libgumbo` is more widely packaged).
- Add a minimal `main.c` that prints usage and exits.
- Add a minimal C test file that passes.
- Update `Makefile` to offer both `make build-go` and `make build-c` (and
  similarly for test), keeping Go as the default for now.
- **Gate:** `cmake --build build && ctest --test-dir build` passes the
  trivial test. Go tests still pass.

### Step 2: Port `packages.go` → `packages.c` + cgo shim

This is the most self-contained module: parse a Debian `Packages` file to
extract package names, and parse a deb source line into URL/dist/component.

- Implement `parse_deb_line()` and `parse_packages_file()` in
  `src/packages.c` with header `src/packages.h`.
- Add them to `libaptfindrepo.a` via CMake.
- Port the 7 test cases from `packages_test.go` to C (reading from the same
  `testdata/packages/` files).
- Implement `fetch_package_list()` using libcurl (fetches Packages.gz,
  gunzips, parses).
- Write `internal/finder/packages_cgo.go`: cgo shim exposing `parseDebLine`
  and `parsePackagesFile` with original Go signatures, calling C.
- **Delete Go:** `packages.go` (replaced by cgo shim) and
  `packages_test.go` (replaced by C tests).
- **Gate:** C package-parsing tests pass. Remaining Go tests still pass
  (validation tests work through the cgo shim).

### Step 3: Port `key.go` → `key.c`

GPG key format detection and normalization.

- Implement `detect_key_format()` and `normalize_key()` in `src/key.c`.
- Add to `libaptfindrepo.a`.
- Port the 9 test cases from `key_normalize_test.go` (reading from
  `testdata/keys/`).
- Implement `fetch_key()` using libcurl.
- **Delete Go:** `key.go` and `key_normalize_test.go` (nothing else in the
  package references key functions — no cgo shim needed).
- **Gate:** C key tests pass. Remaining Go tests still pass.

### Step 4: Port `system.go` → `system.c` + cgo shim

System detection: OS, codename, architecture, Debian→Ubuntu mapping.
Ported before parser and validation because both depend on it — getting the
cgo shim in place early unblocks clean deletion of those later modules.

- Implement `detect_system()` (reads `/etc/os-release`, calls `dpkg
  --print-architecture`) in `src/system.c`.
- Implement `get_ubuntu_codename()` with static Debian→Ubuntu mapping.
- Implement `is_ubuntu_lts()` and `get_nearest_past_lts()`.
- Implement `get_ubuntu_codename_from_api()` API fallback using libcurl +
  a JSON parser (cJSON or json-c).
- Add to `libaptfindrepo.a`.
- Port the test cases from `system_test.go` and
  `system_integration_test.go`.
- Write `internal/finder/system_cgo.go`: cgo shim exposing `SystemInfo`,
  `DetectSystem`, `IsUbuntuLTS`, `GetNearestPastLTS`, `GetUbuntuCodename`,
  `GetUbuntuCodenameFromAPI` with original Go signatures.
- **Delete Go:** `system.go`, `system_test.go`,
  `system_integration_test.go` (replaced by cgo shim + C tests).
- **Gate:** C system tests pass. Remaining Go tests still pass (parser and
  validation tests work through the cgo shim).

### Step 5: Port `parser.go` → `parser.c`

HTML parsing to extract GPG URLs and deb lines. This is the largest and most
complex module.

- Implement `find_gpg_keys()` and `find_deb_lines()` in `src/parser.c`,
  using libgumbo (or lexbor) for HTML DOM traversal, and POSIX regex or
  PCRE2 for pattern matching.
- Implement `is_valid_gpg_url()` filter.
- Implement `find_ppas()` for PPA extraction.
- Implement `dedup()` utility.
- Add to `libaptfindrepo.a`.
- Port the 45 test cases from `repo_finder_test.go` (reading from
  `testdata/webpages/`).
- Port the `TestFindDebLines` synthetic HTML tests.
- Port the `parser_test.go` GPG URL validation tests.
- **Delete Go:** `parser.go`, `parser_test.go`, `repo_finder_test.go` (no
  cgo shim needed — parser functions are only called by `main.go`, which is
  in a separate package and will be ported wholesale in step 7).
- Also delete `system_cgo.go` shim if parser was the last in-package
  consumer (validation still needs it — so keep the shim).
- **Gate:** All C parser tests pass against the same test data. Remaining Go
  tests still pass (`validation_test.go` works through cgo shims for system
  and packages).

### Step 6: Port `validation.go` → `validation.c`

Key-to-source matching, source scoring, and file path generation.

- Implement `match_keys_to_sources()` with domain-based matching in
  `src/validation.c`.
- Implement `score_source()` and `filter_sources_for_system()`.
- Implement `generate_key_path()` and `generate_sources_entry()` (deb822
  format).
- Implement `check_privileges()`, `check_debian_system()`,
  `check_apt_directories()`, `check_conflicts()`.
- Add to `libaptfindrepo.a`.
- Port the test cases from `validation_test.go`.
- **Delete Go:** `validation.go`, `validation_test.go`. Also delete the
  cgo shims (`packages_cgo.go`, `system_cgo.go`) — their last in-package
  consumer is gone. The `internal/finder/` directory is now empty — delete
  it and `internal/`.
- **Gate:** C validation tests pass. No Go code remains in `internal/`.

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
  `go.sum`. No Go source remains anywhere.
- **Gate:** The C binary produces equivalent output to the Go binary for the
  same inputs. `make test` passes with only C.

### Step 8: Reorganize and clean up

All Go is gone. The C code's layout was dictated by the module-by-module
translation. Now we can reorganize freely.

- Reorganize `src/` and `tests/` to the structure we actually want (e.g.,
  consolidate headers, split or merge modules, rename files).
- Simplify `CMakeLists.txt` (drop the static library if not needed — the
  library was scaffolding for cgo linking; a single binary may be simpler).
- Update `Makefile` so `make build` and `make test` target only C (remove
  Go-era and transitional targets).
- Update `CLAUDE.md` and `README.md` to document the C build.
- Update `debian/` packaging if present.
- Remove `.go`-related entries from `.gitignore`.
- **Gate:** `make test` passes. Manual smoke tests pass. Clean repo with
  the layout we want going forward.

## What Gets Deleted When (summary)

| Step | C module added | Go files deleted | Cgo shims |
|------|---------------|-----------------|-----------|
| 2 | `packages.c` | `packages.go`, `packages_test.go` | Add `packages_cgo.go` |
| 3 | `key.c` | `key.go`, `key_normalize_test.go` | (none needed) |
| 4 | `system.c` | `system.go`, `system_test.go`, `system_integration_test.go` | Add `system_cgo.go` |
| 5 | `parser.c` | `parser.go`, `parser_test.go`, `repo_finder_test.go` | (none needed) |
| 6 | `validation.c` | `validation.go`, `validation_test.go`, `packages_cgo.go`, `system_cgo.go`, `internal/` | Remove all shims |
| 7 | `main.c` | `cmd/`, `go.mod`, `go.sum` | — |

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
- **cgo overhead**: The cgo shims are throwaway glue code — small but
  fiddly, especially for struct types like `SystemInfo`. Each shim is
  typically 15–30 lines. The marshaling cost is negligible since this is
  a CLI tool, not a hot loop.
