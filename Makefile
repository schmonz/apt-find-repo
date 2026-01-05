.PHONY: build test clean install

build:
	go build -o apt-find-repo ./cmd/apt-find-repo

test:
	go test -v ./...

clean:
	rm -f apt-find-repo
	rm -rf .cache .go build/*

install: build
	install -D -m 0755 apt-find-repo $(DESTDIR)/usr/bin/apt-find-repo
	install -D -m 0644 docs/apt-find-repo.1 $(DESTDIR)/usr/share/man/man1/apt-find-repo.1

deb:
	dpkg-buildpackage -us -uc -b

man:
	man ./docs/apt-find-repo.1
