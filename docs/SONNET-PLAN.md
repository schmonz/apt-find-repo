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

5. **Delete Go source files one by one using CGo as a bridge.**  When a
   module is ported to C, a thin CGo shim file (`foo_cgo.go`) wraps the C
   functions in the original Go function signatures.  The original `foo.go`
   is then deleted; the Go binary keeps compiling and — crucially — the
   existing Go *tests keep running and now exercise the C code* through the
   shim.  All CGo shims, Go test files, and `main.go` are deleted together
   at Step 8 when `main.c` takes over.

6. **C files live alongside the package during the CGo phase.**  `go build`
   auto-compiles any `.c` files it finds in a CGo-enabled package directory,
   so during Steps 3–7 the C source files live in `internal/finder/`.  The
   libcheck tests in `tests/unit/` reference them there directly.  Step 9
   moves everything to its permanent location.

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
tests/CMakeLists.txt         test harness glue
tests/unit/CMakeLists.txt    libcheck target (zero test cases yet)
```

Note: there is no `src/` subtree yet.  During the CGo phase (Steps 3–7)
the C source files live in `internal/finder/` and the libcheck tests
reference them there.  `src/` is created in Step 9 when the layout is
reorganized.

**CMakeLists.txt requirements:**

- CMake ≥ 3.16, C17 standard
- `find_package(CURL REQUIRED)`
- `find_package(LibXml2 REQUIRED)`
- `pkg_check_modules(CHECK REQUIRED check)` (libcheck)
- `add_subdirectory(tests)` (no `src/` yet)

**Done when:**

```sh
cmake -B build && cmake --build build && ctest --test-dir build
```
exits 0 (vacuously — no test cases yet), and `make test` (Go) is still green.

---

## Step 3: Port Key Detection (`key.c`)

**Go source:** `internal/finder/key.go` (103 lines)
**New C files:** `internal/finder/key.c`, `internal/finder/key.h`
**New CGo shim:** `internal/finder/key_cgo.go`
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

**CGo shim** (`key_cgo.go`) wraps `detect_key_format()` and `normalize_key()`
in the original Go function signatures.  Memory note: `normalize_key` returns
a heap-allocated buffer; the shim must call `C.free` on it after copying into
a Go `[]byte`.

**Go code to delete:** Once `ctest -R key` is green, delete
`internal/finder/key.go`.  The shim `key_cgo.go` keeps the Go package
compiling; the test file `key_normalize_test.go` is kept and now exercises
the C implementation through the shim.

**Done when:** `ctest -R key` passes; `go test ./...` still green (key tests
now run against C code).

---

## Step 4: Port Packages File Parsing (`packages.c`)

**Go source:** `internal/finder/packages.go` (136 lines)
**New C files:** `internal/finder/packages.c`, `internal/finder/packages.h`
**New CGo shim:** `internal/finder/packages_cgo.go`
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

**CGo shim** (`packages_cgo.go`) wraps `parse_packages_file()` and
`parse_deb_line()`.  The shim converts C string arrays to Go slices and
calls `packages_free()`/`deb_line_free()` after copying.

**Go code to delete:** Once `ctest -R packages` is green, delete
`internal/finder/packages.go`.  The shim keeps the package compiling;
`packages_test.go` is kept and now exercises C code.

**Done when:** `ctest -R packages` passes; `go test ./...` still green.

---

## Step 5: Port System Detection (`system.c`)

**Go source:** `internal/finder/system.go` (284 lines)
**New C files:** `internal/finder/system.c`, `internal/finder/system.h`
**New CGo shim:** `internal/finder/system_cgo.go`
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

**CGo shim** (`system_cgo.go`) wraps `get_os_info()` and the codename
mapping functions.  `get_os_info` fills a C struct; the shim copies fields
into a Go struct then calls `os_info_free()`.  The `const char *` returns
from mapping functions are static strings — no freeing needed.

**Go code to delete:** Once `ctest -R system` is green, delete
`internal/finder/system.go`.  The shim keeps the package compiling;
`system_test.go` and `system_integration_test.go` are kept and now
exercise C code.

**Done when:** `ctest -R system` passes; `go test ./...` still green.

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
**New C files:** `internal/finder/parser.c`, `internal/finder/parser.h`
**New CGo shim:** `internal/finder/parser_cgo.go`
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

**CGo shim** (`parser_cgo.go`) wraps `parse_repo_page()`, converting the
returned `parse_result_t` (C arrays of strings) into Go slices, then calling
`parse_result_free()`.  This is the most data-rich shim; take care to copy
all strings before freeing.

**Go code to delete:** Once `ctest -R parser` is green, delete
`internal/finder/parser.go`.  The shim keeps the package compiling;
`parser_test.go` and `repo_finder_test.go` are kept and now exercise C
code across all 45 fixtures.

**Done when:** `ctest -R parser` passes for all 45 fixtures; `go test ./...`
still green.

---

## Step 7: Port Validation and Matching (`validation.c`)

**Go source:** `internal/finder/validation.go` (469 lines)
**New C files:** `internal/finder/validation.c`, `internal/finder/validation.h`
**New CGo shim:** `internal/finder/validation_cgo.go`
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

**CGo shim** (`validation_cgo.go`) wraps matching, filtering, path
generation, and preflight-check functions.  `match_keys_to_sources` returns
a C array of `match_t*`; the shim converts each into a Go struct then calls
`free` on the array.

**Go code to delete:** Once `ctest -R validation` is green, delete
`internal/finder/validation.go`.  The shim keeps the package compiling;
`validation_test.go` is kept and now exercises C code.

At this point all five Go source files in `internal/finder/` have been
replaced by CGo shims.  `main.go` (which imports the `finder` package) still
compiles and the entire `go test ./...` suite still passes — but every test
now runs against C implementations through their respective shims.

**Done when:** `ctest -R validation` passes; `go test ./...` still green
(all tests now exercising C code through CGo shims).

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

**Go code to delete:** Once `make e2e` passes with the C binary, delete all
remaining Go in one commit:

- `cmd/apt-find-repo/main.go` (and the now-empty `cmd/` tree)
- `internal/finder/*_cgo.go` (five CGo shims)
- `internal/finder/*_test.go` (all seven test files)
- `go.mod`, `go.sum`

These can all go together because nothing depends on them any more: the C
binary doesn't use them, the CGo shims were only bridging Go→C, and the
Go test files were only kept to validate those shims.

**Done when:**

1. `build/apt-find-repo -v https://tailscale.com/download/linux/ubuntu-2204`
   produces expected output.
2. `make e2e` passes against the C binary.
3. No Go files of any kind remain in the repository.
4. `ctest --test-dir build` (≡ `make test`) is green.

---

## Step 9: Reorganize the C Layout

**Goal:** Now that Go is fully gone there are no constraints on file layout.
Move everything to wherever makes sense for a C project and update the build
to match.

**Suggested layout** (adjust to taste):

```
src/
  key.c / key.h
  packages.c / packages.h
  system.c / system.h
  parser.c / parser.h
  validation.c / validation.h
  http.c / http.h
  main.c
  CMakeLists.txt      (library + binary targets)
tests/
  unit/
    test_key.c
    test_packages.c
    test_system.c
    test_parser.c
    test_validation.c
    CMakeLists.txt
  e2e/
    run-tests.sh
  CMakeLists.txt
testdata/             (unchanged — still shared with tests)
docs/
  CLAUDE.md           (update to C build instructions)
  apt-find-repo.1
CMakeLists.txt        (root — now adds src/ subdirectory)
Makefile              (thin wrapper around cmake)
```

**Actions:**

1. `git mv internal/finder/*.c internal/finder/*.h src/`
2. Create `src/CMakeLists.txt` with the library and binary targets.
3. Update root `CMakeLists.txt` to `add_subdirectory(src)`.
4. Update `tests/unit/CMakeLists.txt` to reference source files from `src/`
   instead of `internal/finder/`.
5. Delete the now-empty `internal/` directory.
6. Remove `poc/` (proof-of-concept from Step 6a).
7. Update `Makefile`: `build` → `cmake --build build`; `install` → copies
   from `build/`; `clean` → removes `build/`.
8. Update `docs/CLAUDE.md` with C-oriented build and test instructions.
9. Update `README.md` to list C library build dependencies
   (`libcurl4-openssl-dev`, `libxml2-dev`, `check`, cmake).
10. Update `docs/apt-find-repo.1` if build instructions appear there.

**Done when:** `make build && make test && make e2e` all pass; the
repository has no Go files, no `internal/` directory, no `poc/`, and the
C source tree is laid out the way you want it.

---

## Post-Rewrite Follow-ups

These are deferred until after Step 9 (pure C, organized layout).

---

### Follow-up A: BunsenLabs Codename Support

BunsenLabs releases have their own codenames (Lithium, Beryllium, Helium…)
that do not match the underlying Debian codenames they are based on.  The
current `system.c` mapping table only knows Debian↔Ubuntu; running on
BunsenLabs will misidentify the codename and generate wrong `sources.list`
entries.

**Actions:**

1. Extend `get_os_info()` to read `ID_LIKE` from `/etc/os-release` and
   detect BunsenLabs (`ID=bunsenlabs`).
2. Add a BunsenLabs→Debian codename mapping table (Lithium→stretch,
   Beryllium→buster, Helium→bullseye, Boron→bookworm, etc.).
3. When `ID=bunsenlabs`, translate the BunsenLabs codename to the
   underlying Debian codename before passing it to the rest of the logic.
4. Add libcheck test cases feeding synthetic `/etc/os-release` content for
   each BunsenLabs release; assert the correct Debian codename is returned.
5. Add e2e test fixtures (or synthetic invocations) that verify a
   Debian-targeting repo is correctly added on a BunsenLabs host.

---

### Follow-up B: Ubuntu PPA Support on Debian

PPAs (e.g. `ppa:xtradeb/apps`) are an Ubuntu concept but their packages
often build for Debian too.  On a Debian system the tool should:

1. Detect `ppa:owner/name` lines in parsed output (already partially done
   in `parser.c`).
2. Resolve the PPA to its Launchpad HTTPS URL:
   `https://ppa.launchpadcontent.net/<owner>/<name>/ubuntu`
3. Map the Debian codename to the closest Ubuntu LTS codename (already
   handled by `debian_to_ubuntu_codename()`; confirm it covers the needed
   case).
4. Fetch `https://ppa.launchpadcontent.net/<owner>/<name>/ubuntu/dists/<ubuntu-codename>/`
   to verify the PPA actually publishes for that series before writing the
   entry; fall back to the next older LTS if not.
5. Write the `sources.list` entry using the Ubuntu codename and the
   Launchpad key URL.

**Known test case:** `ppa:xtradeb/apps` on Trixie should resolve to the
`noble` (or `jammy`) series, fetch correctly, and install packages.

Add a libcheck unit test with a mock HTTP response for the Launchpad
`InRelease` endpoint, and an e2e fixture that exercises the full path on
a real Debian system (or a docker container).

---

### Follow-up C: Markdown-Based HTML Parsing

The current `parser.c` (ported from Go) uses libxml2 DOM traversal to
find `<code>`/`<pre>` blocks and extract deb lines from them.  This is
fragile: vendor install pages change their markup, and some useful pages
don't use `<code>` blocks at all.

**Proposed approach:** Convert HTML to plain Markdown first, then do dumb
line-by-line pattern matching on the result.

1. Evaluate a C Markdown/HTML-to-text library (e.g. `cmark`, `lowdown`,
   or a simple custom stripper) for converting HTML to plain text /
   Markdown.  A simple approach: strip all tags, collapse whitespace,
   preserve newlines around block elements.
2. Replace the libxml2 DOM logic in `parser.c` with:
   a. HTML → Markdown/plain-text conversion.
   b. Line scan: any line starting with `deb ` or `deb[` is a candidate.
   c. Link scan: any URL-shaped token ending in `.gpg`/`.asc`/`.key`.
3. Run the full 45-fixture test suite against both implementations during
   development; keep whichever approach produces fewer regressions.
4. If the simpler approach wins, drop the libxml2 dependency entirely and
   update `CMakeLists.txt` accordingly.

The goal is robustness over precision: it is better to find the right deb
line on 90 % of real pages than to parse 45 known fixtures perfectly.

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

## What Gets Deleted at Each Step

| Step | Go source deleted        | CGo shim added       | Go tests deleted |
|------|--------------------------|----------------------|------------------|
| 3    | `key.go`                 | `key_cgo.go`         | none             |
| 4    | `packages.go`            | `packages_cgo.go`    | none             |
| 5    | `system.go`              | `system_cgo.go`      | none             |
| 6b   | `parser.go`              | `parser_cgo.go`      | none             |
| 7    | `validation.go`          | `validation_cgo.go`  | none             |
| 8    | `main.go`, all 5 shims   | —                    | all 7            |
| 9    | —                        | —                    | —                |

`go test ./...` stays green from Step 0 through Step 7 (inclusive).  At
Steps 3–7 it runs against C implementations via CGo shims.  Everything Go
is deleted together at Step 8.  `make test` from Step 8 onward is
`ctest --test-dir build` only.
