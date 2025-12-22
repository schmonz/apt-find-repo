# apt-find-repo

Find and add third-party APT repositories from web pages.

## Usage

<https://github.com/JonasGroeger/jetbrains-ppa> is an example of a free-form web page describing a third-party package repository.
Rather than read how to set it up, just see which packages it'd offer you:

```sh
$ apt-find-repo https://github.com/JonasGroeger/jetbrains-ppa
clion
datagrip
goland
intellij-idea-ultimate
pycharm-professional
...
```

Not seeing what you expected? Run with `-v` for more info:

```sh
$ apt-find-repo -v https://github.com/JonasGroeger/jetbrains-ppa
```

Otherwise, add the repo:

```sh
$ sudo apt-find-repo https://github.com/JonasGroeger/jetbrains-ppa add
```

Then install whatever caught your eye:

```sh
$ sudo apt update
$ sudo apt install goland
```


<!--
## Installation
-->

