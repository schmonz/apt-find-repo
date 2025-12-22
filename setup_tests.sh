#!/bin/sh
set -e

mkdir -p testdata/webpages

# Fetch and save test HTML files
curl -s https://github.com/mwt/zoom-apt-repo > testdata/webpages/zoom-unofficial.html
curl -s https://github.com/JonasGroeger/jetbrains-ppa > testdata/webpages/jetbrains-unofficial.html
curl -s https://tailscale.com/download/linux/ubuntu-2204 > testdata/webpages/tailscale-official.html

echo "Test data saved to testdata/webpages/"
echo "Run: go test -v"
