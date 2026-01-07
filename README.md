# apt-find-repo

Automatically find and configure third-party APT repositories.

## Quick Start

Already found docs about a repo?

```sh
$ sudo apt-find-repo jetbrains https://github.com/JonasGroeger/jetbrains-ppa
$ sudo apt update
$ sudo apt install goland
```

Need help finding one?

```sh
$ sudo apt-find-repo tailscale
$ sudo apt update
$ sudo apt install tailscale
```

## Features

- **Security**: Rejects repos without GPG keys, shows its work with `-v`, commits changes with `etckeeper` (if available)
- **Discovery**: Prioritizes official sources over how-to sites
- **Applicability**: Detects distro and arch, maps Debian to nearest Ubuntu, maps Ubuntu non-LTS to nearest LTS as fallback
- **Validity**: Filters incompatible sources, confirms existence of requested package in repo
- **Modernity**: Writes keys to `/etc/apt/keyrings/`, deb822 format to `/etc/apt/sources.list.d/`

## Installation

From source (requires Go 1.21+):

```sh
$ make build
$ sudo make install
```

Build Debian package:

```sh
$ make deb
$ sudo dpkg -i apt-find-repo_*.deb
```

## Requirements

- **Debian-based system** - Ubuntu, Debian, Pop!_OS, etc.
- **root access** - Modifies `/etc/apt/`
- `ddgr` - DuckDuckGo search (`apt install ddgr`)
- `etckeeper` (optional) - Automatic change tracking

## Supported Repository Types

- Official vendor repositories (Docker, Tailscale, Brave, etc.)
- Ubuntu PPAs (ppa:owner/name)
- OpenBuildService repos (build.opensuse.org)
- GitHub-hosted APT repos
- Self-hosted repos with standard structure
