# Testing Infrastructure Analysis

## Current State

### 1. Test Data Coverage ✓
- **45 test pages** in `testdata/webpages/`
- **All documented** in `scripts/setup_tests.sh`
- **All tested** in `internal/finder/repo_finder_test.go`

### 2. Test Data Freshness ⚠️
Tested 4 pages by re-fetching:
- tailscale: no changes ✓
- brave: no changes ✓
- docker: **CHANGED** ⚠️
- grafana: **CHANGED** ⚠️

**Conclusion**: Pages do change over time. We need:
- Regular re-fetching to catch breakage
- Version pinning OR expectation that tests may need updating
- CI job to detect when pages change

### 3. GPG Key Validation ⚠️

**Current criteria** (`internal/finder/key.go:detectKeyFormat`):
1. Binary format: First byte is 0x99 or 0x80-0xBF
2. Armored format: Contains "-----BEGIN PGP PUBLIC KEY BLOCK-----"
3. Dearmored: >30% non-printable bytes

**Risks**:
- ❌ **False positives**:
  - bash expansion patterns like `install.sh{,.asc` (found in brave-official)
  - Multi-line yum config strings (found in vscode-official)
  - These get matched by our URL regex but aren't valid keys

- ❌ **False negatives**:
  - Keys with unusual extensions (.pub, .txt)
  - Keys served with wrong Content-Type
  - Truncated/corrupted downloads

**Current tests**: 6 test cases in `key_normalize_test.go`
- armored-with-headers
- armored-no-headers
- binary-gpg
- dearmored-output
- html-wrapped
- not-a-key (garbage)

**Missing tests**:
- Keys with `.pub` extension
- Keys with no extension
- Truncated armored keys
- Invalid armored keys (wrong headers)
- Binary keys with wrong magic bytes
- Zero-byte files
- HTML error pages (404, 500)

### 4. Source Line Selection ❌ CRITICAL GAP

**Current logic**: NO system-specific filtering!

`MatchKeysToSources` simply:
1. Groups keys and sources by domain
2. Returns ALL matches
3. Main uses `matches[0]` (first match)

**Problems**:
1. ❌ No version filtering (jammy vs focal vs bookworm)
2. ❌ No architecture filtering (amd64 vs arm64)
3. ❌ No component filtering (testing vs stable)
4. ❌ Arbitrary selection (just picks first)

**Example from Tailscale**:
```go
// We find these:
"deb [signed-by=...] https://pkgs.tailscale.com/stable/ubuntu jammy main"
"deb [signed-by=...] https://pkgs.tailscale.com/stable/ubuntu focal main"

// We pick: matches[0] (could be wrong version!)
```

**What we should do**:
1. Detect running system: `lsb_release -cs` or `/etc/os-release`
2. Detect architecture: `dpkg --print-architecture`
3. Filter sources that match our system
4. Prefer: exact version match > same OS > any
5. Handle `$(dpkg --print-architecture)` in source lines

**Missing tests**:
- Multiple sources for same repo (different versions)
- Multiple sources for same repo (different architectures)
- Sources with shell expansions `$(dpkg ...)`
- Sources that don't match running system
- Sources with multiple components

## Action Items

### High Priority (Blocking)
1. [ ] Add system-aware source selection
2. [ ] Add tests for source selection
3. [ ] Filter out false positive "keys" (bash expansions, configs)

### Medium Priority
4. [ ] Add more GPG key validation tests
5. [ ] Add CI job to detect webpage changes
6. [ ] Document expected test maintenance

### Low Priority
7. [ ] Consider caching test data with timestamps
8. [ ] Add test for "no valid sources for this system"
