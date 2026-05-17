## Code & Docs Review: `apt-find-repo`

### Overview

A Go CLI tool that discovers third-party APT repositories by scraping web pages, matching GPG keys to `deb` source lines, validating packages exist, and writing to `/etc/apt/`. The core architecture is sound; the library split into `internal/finder/` is good. Issues are concentrated in three areas: test correctness, main.go bloat, and stale documentation.

---

### Bugs

**`stringSlicesEqual()` uses substring containment, not equality** (`repo_finder_test.go:462–471`)

```go
if !strings.Contains(a[i], b[i]) && !strings.Contains(b[i], a[i]) {
```

`strings.Contains` is not equality. `"https://a.com/key.gpg"` would match `"https://a.com/key.gpg/extra"`. Swap for `a[i] != b[i]`. This masks real regressions in the 40+ HTML-parsing test cases.

**`TestParsePackagesFile` silently passes mismatches when either slice is empty** (`packages_test.go:61`)

```go
if !reflect.DeepEqual(packages, tc.wantPackages) && len(packages) != 0 && len(tc.wantPackages) != 0 {
```

The extra `&&` conditions mean if `parsePackagesFile` returns `[]` when it should return packages (or vice versa), the test passes. Drop the two extra clauses; let `reflect.DeepEqual` do the job, and handle the `empty.txt` case by expecting an empty slice normally.

**`TestNormalizeKeyEdgeCases` expects truncated/malformed armored keys to fail, but `NormalizeKey` succeeds on them** (`key_normalize_test.go:152–194` vs `key.go:50–73`)

`detectKeyFormat` returns `"armored"` for any data containing `"-----BEGIN PGP PUBLIC KEY BLOCK-----"`, then `extractArmoredKey` succeeds as long as the `BEGIN`/`END` markers are both present. A `truncated.asc` that has both markers but garbled body will pass through `NormalizeKey` without error, yet the test marks `wantError: true`. The test is documenting desired behavior that isn't implemented — either validate armored key structure (e.g. try to parse with `golang.org/x/crypto/openpgp`) or update the test comment to reflect what actually happens.

---

### Dead Code / Unused Variables

**`extractRepoName()` is never called** (`main.go:732–747`). Should be deleted.

**`url = result.url` at `main.go:299`** assigns `url` a value it already holds from the candidate loop, and `url` is not read after this point. Dead assignment.

---

### Code Quality

**`GenerateSourcesEntry()` component extraction is convoluted** (`validation.go:263–273`)

The loop to find components uses `strings.HasPrefix(part, "http")` in the outer `if` but that branch never sets `foundFirst = true`, making it dead logic. The whole loop can be replaced: call `parseDebLine()` to strip the options block and recover the URL, then use `strings.Fields` on the remainder to grab components directly. The existing tests pass by coincidence and would likely miss edge cases with unusual spacing or options.

**`parseInstallScript()` in `main.go` duplicates logic from `internal/finder/parser.go`** (`main.go:613–729`). Both functions extract GPG URLs and deb lines from text using similar regex patterns. The install-script parser lives in `main.go` where it cannot be unit-tested, while the nearly-identical HTML parser lives in the library where it can. Moving `parseInstallScript`, `findInstallScripts`, `searchForRepo`, and `validatePackageGlob` into the `finder` package would enable testing and eliminate the duplication.

**`findInstallScripts()` has redundant patterns** (`main.go:573–583`). The patterns for `/install.sh` are strict subsets of the patterns for `*.sh`. The four-pattern set collapses to two.

**`main()` is ~370 lines** and handles HTTP fetching, HTML parsing, install-script fetching, PPA resolution, system detection, file writing, and etckeeper integration. The PPA and install-script branches especially should be encapsulated — preferably in `internal/finder/` where they can be tested.

**No HTTP timeouts anywhere** in `packages.go`, `parser.go`, `key.go`, `system.go`. All calls use default `http.Get()`. On a slow or unresponsive server the tool hangs indefinitely. Add a shared `http.Client` with a `Timeout` of something reasonable (e.g. 30s for key/package fetches, shorter for the Launchpad API).

**No partial-write rollback** (`main.go:346–363`). If the key write succeeds but the sources file write fails, the orphaned key file is left behind. A simple `defer os.Remove(keyPath)` that's cancelled on success would fix this.

**`FetchPackageList` hardcodes `["amd64", "all"]`** (`packages.go:49`). It doesn't use the detected system architecture. On `arm64` systems it would fetch the wrong (or no) package list.

**`GetNearestPastLTS()` silently returns `"noble"`** for unknown non-LTS codenames (`system.go:170`). This is a silent best-guess fallback that could install the wrong repository. Better to return an error or require explicit handling.

---

### Testability

**All HTTP calls are in concrete functions with no interface seam.** `FetchKey`, `FetchPackageList`, `FetchPPAInfo`, and `GetUbuntuCodenameFromAPI` make real network calls. This makes unit testing of any function that calls them impossible without network. Introduce an `HTTPClient` interface (or accept an `*http.Client` parameter) so tests can inject a `httptest` server.

**`parseInstallScript()` and `searchForRepo()` in `main.go` have zero unit tests.** The `searchForRepo` blacklist logic and the shell-variable substitution in `parseInstallScript` are particularly intricate and need tests.

**`TestCheckConflicts` has a TODO placeholder** (`validation_test.go:299`): `"We'll skip actual file creation tests for now since they need temp directories"`. `os.MkdirTemp` exists for exactly this.

**Integration tests make live network calls without `testing.Short()` guards** (`system_integration_test.go:102` does use `Short()`, but `TestCodenameMapping` at line 10 calls `DetectSystem()` which shells out to `dpkg` — this'll fail on non-Debian CI runners without a clear skip condition).

---

### Documentation

**`CLAUDE.md` describes a different version of the tool.** It says the tool takes a URL argument and has "list mode" (default) and "add mode" (`add` argument). The actual interface is `<package-glob> [url]` with no separate add mode — it always adds. The module breakdown also references old lowercase function names. This will actively mislead contributors.

**`TESTING.md` is contradicted by the current code.** The "Source Line Selection — CRITICAL GAP" section says there is no system-aware filtering, but `FilterSourcesForSystem`, `MatchKeysToSourcesWithSystem`, and their tests clearly implement this. The document should either be updated to reflect current state or deleted (per the TODO.md item).

**`README.md` doesn't mention `ddgr`'s role in the auto-discovery path.** The Requirements section mentions it, but the Quick Start examples don't distinguish which mode needs it. A new user running `sudo apt-find-repo tailscale` without `ddgr` installed will get an unhelpful failure.

---

### Ease of Use

**Non-verbose "no working repository found" error** (`main.go:295`) gives no diagnostic information. Verbose mode (`-v`) is excellent for debugging, but the default stderr output for the auto-discovery path should at minimum print how many candidates were tried.

**Silent wildcard append** (`main.go:44–47`). `tailscale` becomes `tailscale*` automatically. This is noted in usage output but could surprise users who expect exact matching (e.g., looking for a package literally named `tailscale`). A note in the error output or a `--exact` flag would help.

**The search blacklist** (`main.go:445–546`) is 60+ manually maintained domain strings. New tutorial sites appear constantly and won't be blocked until noticed and listed. Consider inverting to an allowlist of known good domains, or scoring results by domain reputation rather than hard-blocking.

---

### Summary Table

| Area | Severity | Count |
|---|---|---|
| Test correctness bugs | High | 2 |
| Dead code | Low | 2 |
| No HTTP timeouts | Medium | 1 |
| No rollback on partial write | Medium | 1 |
| Hardcoded architecture in package fetch | Medium | 1 |
| Untested functions in main.go | Medium | 3 |
| Stale docs (CLAUDE.md, TESTING.md) | Medium | 2 |
| Code duplication (parser logic) | Low | 1 |
| Convoluted component extraction | Low | 1 |

The highest-priority fixes are the two test correctness bugs (`stringSlicesEqual` and the empty-slice guard in `TestParsePackagesFile`), which are currently hiding potential regressions in the parser, and updating `CLAUDE.md` to match the actual interface.
