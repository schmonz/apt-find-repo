# TODO

- [ ] Omit Ubuntu maybe from search terms
- [ ] Tighten how we match on install script
      and don't follow arbitrarily deep. just once
- [ ] Filter out a bunch of crummy tutoril sotes
- [ ] Make aure we actually have enough results left to iterate over
- [ ] What happens with actual glob characyers?
- [ ] Can we try to match source against our running system?
      "noble" and "and64" or whatever
      when would we reject a source that otherwise matches?
- [ ] Make sure we have microtests for all these edges
- [ ] Does it simplify our code if we first reduce HTML to Markdown?
- [ ] Handle a bunch more examples
- [ ] Rewrite in C with CMake, libcheck, freely taking library dependencies to avoid reinventing any wheels (json, regex, markdown, even a vendored copy of ddgr if that's how we can use it as a library)
- [ ] Create Debian packages with OpenBuildService
- [ ] Publish a Homebrew tap
