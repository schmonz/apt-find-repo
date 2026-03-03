# Plan: Incremental C Rewrite of apt-find-repo

## Background and Motivation

The current implementation is ~2,900 lines of Go across six modules with one
external library dependency (goquery). The rewrite target is C with:

- **CMake** as the build system
- **libcurl** for HTTP fetching
- **libxml2** for HTML parsing (see Step 6a for discussion)
- **libcheck** for unit testing (already listed in TODO.md)
- **getopt\_long** (glibc) for CLI argument parsing
- Additional libraries as needed (PCRE2, jansson/cJSON)

The 60 testdata fixtures (45 cached webpages, 9 GPG key files, 7 Packages
files) are reused verbatim in C tests — no changes to `testdata/` are needed
at any step.

## Guiding Principles

1. **Green at every step.** After each step, all existing tests must pass
   before proceeding.  In steps where only C code is being added, the Go
   test suite (`make test`) must still be green.  In steps where C replaces
   Go, the C test suite takes over.

2. **Bottom-up porting.** Port leaf modules (no internal deps) first, then
   modules that depend on them.

3. **Same testdata.** The libcheck C tests read from the same `testdata/`
   directories the Go tests use.  No fixture duplication.

4. **One module at a time.** Each numbered step ports one Go source file to
   one C compilation unit.

5. **Remove Go code last.** Go source stays in the tree until the C binary
   is fully working and covered by tests.

## Module Dependency Map

```
key.go          (no internal deps)
system.go       (no internal deps)
packages.go     (no internal deps; HTTP deferred to Step 8)
parser.go       (no internal deps; HTTP deferred to Step 8)
validation.go   (depends on: system.go, parser.go, key.go)
main.go         (depends on: everything + HTTP + ddgr subprocess)
```

Porting order: **key → system → packages → parser → validation → main**

---

## Step 0: Refresh Test Data and Establish a Green Baseline

**Goal:** Make `go test ./...` pass against freshly-fetched upstream content.

**Actions:**

```sh
sh scripts/setup_tests.sh   # re-fetches all 45 cached HTML pages
make test                   # must be green before anything else
```

**Expected complications:** Docker and Grafana pages are known to change
(per TESTING.md). If any test fails due to changed page content, update
the test expectations in `internal/finder/repo_finder_test.go` and commit
the updated fixture + expectation together.

**Done when:** `go test ./...` exits 0 with no skipped tests.

---

## Step 1: Add a Shell-Based End-to-End Test Script

**Goal:** Create a language-agnostic integration test harness that validates
the compiled binary's behaviour end-to-end.  This script will be the
primary "things still work" gate once Go code is removed.

**Rationale:** The existing `go test ./...` suite tests Go code, not the
binary. When we switch to C, we need a test harness that survives the
language change.

**Actions:**

1. Create `tests/e2e/run-tests.sh`.
2. The script compiles the binary (`make build`), then invokes it with a
   set of known inputs and checks that outputs match expectations.
3. For each testdata webpage, use a `--local-file <path>` flag (to be added
   to the Go binary in this step) to parse from a local file rather than
   fetching over the network.
4. Assert: correct GPG key URLs found, correct deb-source lines found,
   correct package names returned.
5. Add `make e2e` target to Makefile.

**Done when:** `make e2e` passes against the Go binary; `make test` still
passes.

---

## Step 2: Set Up C Build Infrastructure

**Goal:** Introduce CMake and libcheck alongside the Go build; no C logic yet.

**Files to create:**

```
CMakeLists.txt               root CMake config
src/CMakeLists.txt           library + binary targets (stubs only)
src/apt_find_repo.h          shared typedefs, forward declarations
tests/CMakeLists.txt         test harness glue
tests/unit/CMakeLists.txt    libcheck target (zero test cases yet)
```

**CMakeLists.txt requirements:**

- CMake ≥ 3.16, C17 standard
- `find_package(CURL REQUIRED)`
- `find_package(LibXml2 REQUIRED)`
- `pkg_check_modules(CHECK REQUIRED check)` (libcheck)
- `add_subdirectory(src)` and `add_subdirectory(tests)`

**Done when:**

```sh
cmake -B build && cmake --build build && ctest --test-dir build
```
exits 0 (vacuously — no test cases yet), and `make test` (Go) is still green.

---

## Step 3: Port Key Detection (`key.c`)

**Go source:** `internal/finder/key.go` (103 lines)
**New files:** `src/key.c`, `src/key.h`
**New test:** `tests/unit/test_key.c`

**Public API to implement:**

```c
typedef enum {
    KEY_FORMAT_ARMORED,    /* ASCII-armored PGP block */
    KEY_FORMAT_BINARY,     /* raw binary OpenPGP */
    KEY_FORMAT_DEARMORED,  /* binary without OpenPGP framing */
    KEY_FORMAT_UNKNOWN,
} key_format_t;

key_format_t detect_key_format(const uint8_t *data, size_t len);

/* Returns 0 on success; caller frees *out with free(). */
int normalize_key(const uint8_t *in, size_t in_len,
                  uint8_t **out, size_t *out_len);
```

**Detection heuristics (mirroring Go):**

- Binary: first byte is 0x99 or in range 0x80–0xBF
- Armored: contains `-----BEGIN PGP PUBLIC KEY BLOCK-----`
- Dearmored: > 30 % non-printable bytes (and not binary by above rule)
- Otherwise: UNKNOWN

**libcheck test cases** (one per fixture in `testdata/keys/`):

| Fixture              | Expected format   | normalize result        |
|----------------------|-------------------|-------------------------|
| `armored-full.asc`   | ARMORED           | pass-through            |
| `armored-stripped.asc` | ARMORED         | pass-through            |
| `binary.gpg`         | BINARY            | pass-through            |
| `dearmored.gpg`      | DEARMORED         | wrap in PGP headers     |
| `empty.gpg`          | UNKNOWN           | error                   |
| `garbage.txt`        | UNKNOWN           | error                   |
| `html-wrapped.txt`   | ARMORED           | strip HTML then pass    |
| `malformed.asc`      | UNKNOWN           | error                   |
| `truncated.asc`      | UNKNOWN           | error                   |

**Done when:** `ctest -R key` passes; `make test` (Go) still green.

---

## Step 4: Port Packages File Parsing (`packages.c`)

**Go source:** `internal/finder/packages.go` (136 lines)
**New files:** `src/packages.c`, `src/packages.h`
**New test:** `tests/unit/test_packages.c`

**Public API to implement:**

```c
typedef struct {
    char **names;   /* NULL-terminated array of package name strings */
    size_t count;
} packages_t;

/* Parse Debian Packages file content; returns NULL on error. */
packages_t *parse_packages_file(const char *data, size_t len);
void        packages_free(packages_t *p);

/* Parse a single "deb" source line into its components. */
typedef struct {
    char *url;
    char *distribution;
    char *component;
    char *options;   /* e.g. "[signed-by=...]" */
} deb_line_t;

deb_line_t *parse_deb_line(const char *line);
void        deb_line_free(deb_line_t *d);
```

HTTP fetching of `Packages.gz` is deferred to Step 8.  Tests read fixtures
directly from `testdata/packages/`.

**libcheck test cases** (one per fixture):

| Fixture              | Expected result                                  |
|----------------------|--------------------------------------------------|
| `empty.txt`          | zero packages                                    |
| `simple.txt`         | known short package list                         |
| `multiarch.txt`      | packages from multiple architectures             |
| `with-source.txt`    | package list (source stanzas ignored)            |
| `jetbrains-real.txt` | large real-world file, spot-check package names  |
| `tailscale-real.txt` | spot-check `tailscale` is present                |
| `zoom-real.txt`      | spot-check `zoom` is present                     |

**Done when:** `ctest -R packages` passes; `make test` (Go) still green.

---

## Step 5: Port System Detection (`system.c`)

**Go source:** `internal/finder/system.go` (284 lines)
**New files:** `src/system.c`, `src/system.h`
**New test:** `tests/unit/test_system.c`

**Public API to implement:**

```c
typedef struct {
    char *os_id;        /* e.g. "ubuntu", "debian" */
    char *codename;     /* e.g. "jammy", "bookworm" */
    char *arch;         /* e.g. "amd64", "arm64" */
} os_info_t;

int         get_os_info(os_info_t *info);  /* reads /etc/os-release + dpkg */
void        os_info_free(os_info_t *info);
const char *debian_to_ubuntu_codename(const char *debian_codename);
const char *lts_fallback(const char *codename);
int         is_lts_codename(const char *codename);
```

The Debian↔Ubuntu codename mapping is a hardcoded lookup table matching the
Go implementation.  The API-based lookup (Ubuntu launchpad) is a low-priority
follow-up; the table suffices for current test coverage.

**libcheck test cases** (mirror `internal/finder/system_test.go`):

- All Debian→Ubuntu codename mappings (trixie→noble, bookworm→jammy, etc.)
- LTS codename detection (noble, jammy, focal are LTS; oracular, mantic are not)
- LTS fallback logic for non-LTS Ubuntu releases
- `/etc/os-release` parsing (feed a synthetic file, not the live system file)

**Done when:** `ctest -R system` passes; `make test` (Go) still green.

---

## Step 6a: Choose the HTML Parsing Strategy

**Goal:** Decide on the C approach for HTML parsing before writing parser.c.

**Options:**

1. **libxml2 (HTML mode) + XPath** — Parse the DOM properly.  Use XPath
   expressions to locate `<code>`, `<pre>`, and `<a href>` elements, then
   extract text content.  Most correct; widely packaged; somewhat verbose.

2. **PCRE2 on raw HTML** — Apply regular expressions directly to the raw
   HTML bytes.  Matches the spirit of the existing Go code ("intentionally
   permissive").  Avoids a significant C dependency but is fragile if
   markup is unusual.

3. **Hybrid: libxml2 for text extraction, then regex** — Use libxml2 in
   HTML mode to extract plain text from `<code>` and `<pre>` blocks, then
   run simple string-search / POSIX regex on that text.  Best of both
   worlds: correct scoping (only looks inside code blocks) without needing
   complex XPath.

**Recommendation:** Option 3 (hybrid).  libxml2 is already needed to
correctly scope the search to code blocks (which the Go goquery code does
via CSS selectors), and using simple string matching on the extracted text
avoids the complexity of full PCRE2 for the inner patterns.

**Action:** Build a small proof-of-concept (`poc/parse_poc.c`) that extracts
text from `<code>` and `<pre>` blocks using libxml2, run it against five
diverse webpages from `testdata/webpages/`, and confirm the extracted text
contains the expected deb lines.  Adjust strategy if the PoC reveals issues.

**Done when:** PoC succeeds on at least Docker, Tailscale, Syncthing,
Kubernetes, and a Launchpad PPA page.

---

## Step 6b: Port HTML Parser (`parser.c`)

**Go source:** `internal/finder/parser.go` (284 lines)
**New files:** `src/parser.c`, `src/parser.h`
**New test:** `tests/unit/test_parser.c`

**Public API to implement:**

```c
typedef struct {
    char **key_urls;    /* NULL-terminated; URLs ending in .gpg/.asc/.key */
    int    key_count;
    deb_line_t **deb_lines;
    int    deb_count;
} parse_result_t;

parse_result_t *parse_repo_page(const char *html, size_t len,
                                const char *base_url);
void            parse_result_free(parse_result_t *r);
```

**Extraction logic (mirroring Go):**

- GPG key URLs: links (`<a href>`, `<code>`, `<pre>`) whose path ends in
  `.gpg`, `.asc`, or `.key`, excluding shell expansion artefacts like
  `{,.asc}`.
- Deb source lines: lines beginning with `deb ` or `deb[` found inside
  `<code>` and `<pre>` blocks.
- Fallback: follow URLs that look like `.list` files referenced in
  `curl`/`wget` commands and parse their content.
- PPA detection: lines of the form `ppa:owner/name`.

**libcheck test cases:** Cover all 45 `testdata/webpages/` fixtures,
asserting the same key URLs and deb lines as the Go
`TestParseRepoPage` table.  This is the largest single test file.
Break into sub-cases by category (official docs, GitHub READMEs, Launchpad)
if the file becomes unwieldy.

**Done when:** `ctest -R parser` passes for all 45 fixtures; `make test`
(Go) still green.

---

## Step 7: Port Validation and Matching (`validation.c`)

**Go source:** `internal/finder/validation.go` (469 lines)
**New files:** `src/validation.c`, `src/validation.h`
**New test:** `tests/unit/test_validation.c`

**Public API to implement:**

```c
typedef struct {
    char      *key_url;
    deb_line_t *source;
} match_t;

match_t   **match_keys_to_sources(char **key_urls, int key_count,
                                   deb_line_t **sources, int src_count,
                                   int *out_count);
deb_line_t **filter_sources_for_system(deb_line_t **sources, int count,
                                        const os_info_t *os,
                                        int *out_count);
char       *generate_key_path(const char *repo_name, key_format_t fmt);
char       *generate_sources_entry(const match_t *m, const char *key_path);

int check_privileges(void);
int check_debian_system(void);
int check_apt_directories(void);
int check_conflicts(const char *key_path, const char *sources_path);
```

**libcheck test cases** (mirror `internal/finder/validation_test.go`):

- Domain-based matching (single key + single source, same domain)
- Multi-source disambiguation (prefer exact codename match)
- Path-based matching fallback
- Ambiguity: two keys, two sources, different domains
- System-aware filtering with mock `os_info_t`
- `generate_key_path` and `generate_sources_entry` round-trips

**Done when:** `ctest -R validation` passes; `make test` (Go) still green.

---

## Step 8: Port HTTP Fetching and Main Orchestration (`main.c`)

**Go source:** `cmd/apt-find-repo/main.go` (794 lines)
**New files:** `src/http.c`, `src/http.h`, `src/main.c`

**HTTP module (`http.c`):**

```c
typedef struct {
    uint8_t *body;
    size_t   len;
    long     status_code;
    char    *content_type;
} http_response_t;

http_response_t *http_fetch(const char *url);
void             http_response_free(http_response_t *r);
```

Wraps libcurl (`curl_easy_*`).  Reuse for both HTML page fetching and GPG
key downloading.  Handles gzip decompression (via libcurl's built-in
`CURLOPT_ACCEPT_ENCODING`).

**CLI interface** (`main.c`, must match the existing Go CLI exactly):

```
apt-find-repo [-v] <url> [add]
```

- Default (list) mode: fetch page → parse → match → fetch package list →
  print package names.
- Add mode: list mode + write key to `/etc/apt/keyrings/` and sources entry
  to `/etc/apt/sources.list.d/` (requires root).

**ddgr integration:** When a search term rather than a URL is provided,
invoke ddgr as a subprocess (`popen` or `fork`/`exec`) with the same
blacklist of low-quality sites maintained in Go.

**Decompression of `Packages.gz`:** Use libcurl's built-in support; fall
back to trying the uncompressed URL if `.gz` fetch fails (matching Go logic).

**Done when:**

1. `cmake --build build && build/apt-find-repo -v https://tailscale.com/download/linux/ubuntu-2204`
   produces expected output.
2. `make e2e` (shell end-to-end tests from Step 1) passes against the C
   binary.
3. `make test` (Go) still green (both codebases coexist at this point).

---

## Step 9: Switch Over and Remove Go Code

**Goal:** Make C the default; retire all Go source.

**Actions (in order):**

1. Update `Makefile`:
   - `build` target runs `cmake --build build` instead of `go build`
   - `test` target runs `ctest --test-dir build` instead of `go test ./...`
   - `install` target copies `build/apt-find-repo` instead of the Go binary
   - `clean` removes `build/` instead of Go artifacts
2. Delete `cmd/` (Go entry point).
3. Delete `internal/` (Go library modules).
4. Delete `go.mod` and `go.sum`.
5. Update `docs/CLAUDE.md` with C-oriented build and test instructions.
6. Update `README.md` to list C library build dependencies.
7. Update `docs/apt-find-repo.1` if build instructions appear in the man page.
8. Remove `poc/` (proof-of-concept from Step 6a).

**Done when:** `make build && make test && make e2e` all pass using only
the C toolchain.  No Go files remain.

---

## Library Dependency Summary

| Library    | Purpose                          | Debian package         |
|------------|----------------------------------|------------------------|
| libcurl    | HTTP fetching, gzip decompression | `libcurl4-openssl-dev` |
| libxml2    | HTML DOM parsing (Step 6b)        | `libxml2-dev`          |
| libcheck   | C unit test framework             | `check`                |
| (optional) PCRE2 | Regex if libxml2 proves insufficient | `libpcre2-dev` |
| (optional) jansson | JSON for Ubuntu launchpad API | `libjansson-dev` |

All are available from the standard Debian/Ubuntu archive and can be listed
as `Build-Depends` in the eventual `debian/control` file.

---

## Test Coverage at Each Step

| Step | Test command                      | What it covers                     |
|------|-----------------------------------|------------------------------------|
| 0    | `make test`                       | Go baseline, refreshed fixtures    |
| 1    | `make test && make e2e`           | Go + shell e2e                     |
| 2    | `make test && ctest --test-dir build` | Go + empty C build             |
| 3    | `make test && ctest -R key`       | Go + C key detection               |
| 4    | `make test && ctest -R packages`  | Go + C package parsing             |
| 5    | `make test && ctest -R system`    | Go + C system detection            |
| 6a   | `make test`                       | Go (PoC manual inspection)         |
| 6b   | `make test && ctest -R parser`    | Go + C HTML parsing (45 fixtures)  |
| 7    | `make test && ctest -R validation`| Go + C matching & filtering        |
| 8    | `make test && make e2e`           | Go + C binary e2e                  |
| 9    | `make test && make e2e`           | C only                             |
