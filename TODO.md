# TODO

## 2. Narrow results "enough"

- Try to match source against our running system? ("noble" and "amd64", or whatever)
    - What if there are multiple source entries to choose from?
    - What if there's only one source entry to choose from?
    - When should we reject a source that otherwise matches?

## 3. Test existing behavior "completely"

- Make sure all the edges are microtested
- Investigate:
    - Do some of our example webpages change every fetch?
    - Do some of our example webpages have JavaScript that make them useless if we don't execute it? (We won't, and maybe we should error meaningfully)

## 4. Get realer

- Launchpad PPAs
- GitHub (if this is somehow generically meaningful)
- Other popular packages
- Less popular packages whose webpages we can't parse yet

## 5. Prepare for maintenance

- GitHub Actions workflow:
    - On every push: build and microtest
    - On tags: integration-test/build/microtest/release
    - By manual request: integration-test (fetch fresh testdata, fail if any diffs)
- Packaging:
    - Debian with OpenBuildService (so there's a repo!)
        - Also available from GitHub Releases, with GPG key and source list included? (Because they don't have `apt-find-repo` yet!)
    - Homebrew tap
    - pkgsrc (of course of course)
- Rewrite in C:
    - CMake
    - libcheck
    - program dependency on `ddgr`
    - library dependencies for json, regex, whatever else
- Experimental cleanups:
    - If we reduce HTML to Markdown before parsing, does that simplify our code?
    - Where else is there duplication or other opportunity for confusion?
    - Where else can we get smaller and simpler?
- Docs:
    - Tighten manpage and `README.md`
    - Remove this `TODO.md`, maybe also `CLAUDE.md` and `TESTING.md`
