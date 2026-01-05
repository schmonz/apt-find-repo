#!/bin/sh
set -e

mkdir -p testdata/keys

# Fetch real armored key
curl -s https://mirror.mwt.me/zoom/gpgkey > testdata/keys/armored-full.asc

# Create armored without extra whitespace
grep -v '^$' testdata/keys/armored-full.asc > testdata/keys/armored-stripped.asc

# Fetch binary key
curl -s https://pkgs.tailscale.com/stable/ubuntu/jammy.noarmor.gpg > testdata/keys/binary.gpg

# Create dearmored version (requires gpg)
if command -v gpg >/dev/null 2>&1; then
    gpg --dearmor < testdata/keys/armored-full.asc > testdata/keys/dearmored.gpg 2>/dev/null
else
    # Fallback: copy binary key as dearmored (same format)
    cp testdata/keys/binary.gpg testdata/keys/dearmored.gpg
fi

# Create HTML-wrapped key
cat > testdata/keys/html-wrapped.txt <<'EOF'
<!DOCTYPE html>
<html>
<body>
<h1>Install Instructions</h1>
<pre>
-----BEGIN PGP PUBLIC KEY BLOCK-----

mQINBGLvAj0BEADFqyP9to6M7cJ7RnNvGJE0FkJFxQqQKvVBNaW8dCqJN8CcZSWx
Fake key data here for testing
-----END PGP PUBLIC KEY BLOCK-----
</pre>
</body>
</html>
EOF

# Create garbage file
echo "This is not a PGP key at all!" > testdata/keys/garbage.txt

echo "Key test data created in testdata/keys/"
echo "Run: go test -v -run TestDetectKeyFormat"
