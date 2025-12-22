#!/bin/sh
set -e

mkdir -p testdata/packages

# Simple Packages file
cat > testdata/packages/simple.txt <<'EOF'
Package: package-a
Version: 1.0
Architecture: amd64

Package: package-b
Version: 2.0
Architecture: all

Package: package-c
Version: 1.5
Architecture: amd64
EOF

# With Source field (should be ignored)
cat > testdata/packages/with-source.txt <<'EOF'
Package: real-package
Version: 1.0
Architecture: amd64

Source: source-only-pkg
Binary: real-package

Package: another-package
Version: 2.0
Architecture: all
EOF

# Multi-arch
cat > testdata/packages/multiarch.txt <<'EOF'
Package: amd64-pkg
Version: 1.0
Architecture: amd64

Package: arm64-pkg
Version: 1.0
Architecture: arm64

Package: all-arch-pkg
Version: 1.0
Architecture: all
EOF

# Empty file
touch testdata/packages/empty.txt

# Fetch real Packages files for testing
echo "Fetching real Packages files..."

# Zoom repo
curl -s https://mirror.mwt.me/zoom/deb/dists/any/main/binary-amd64/Packages > testdata/packages/zoom-real.txt

# JetBrains PPA
curl -s http://jetbrains-ppa.s3-website.eu-central-1.amazonaws.com/dists/any/main/binary-amd64/Packages > testdata/packages/jetbrains-real.txt

# Tailscale (try .gz first, fall back to uncompressed)
if curl -s https://pkgs.tailscale.com/stable/ubuntu/dists/jammy/main/binary-amd64/Packages.gz | gunzip > testdata/packages/tailscale-real.txt 2>/dev/null; then
    echo "  Fetched Tailscale (gzipped)"
else
    curl -s https://pkgs.tailscale.com/stable/ubuntu/dists/jammy/main/binary-amd64/Packages > testdata/packages/tailscale-real.txt
    echo "  Fetched Tailscale (uncompressed)"
fi

echo "Package test data created in testdata/packages/"
echo "Run: go test -v -run Packages"
