#!/bin/sh
# Analyze what can be extracted from each testdata HTML file

for file in testdata/webpages/*.html; do
    if [ ! -s "$file" ]; then
        echo "SKIP (empty): $(basename $file)"
        continue
    fi

    name=$(basename "$file" .html)

    # Count GPG key URLs
    gpg_count=$(grep -oE "https?://[^\"\' <>]+\.(gpg|asc|key)" "$file" 2>/dev/null | grep -v "^//" | wc -l)

    # Count deb lines
    deb_count=$(grep -oE "deb(\-src)? (\[[^\]]+\] )?https?://[^ ]+ [a-z0-9][a-z0-9._-]+ [a-z]+" "$file" 2>/dev/null | wc -l)

    # File size
    size=$(ls -lh "$file" | awk '{print $5}')

    if [ $gpg_count -gt 0 ] || [ $deb_count -gt 0 ]; then
        printf "%-30s  GPG:%-2d  DEB:%-2d  SIZE:%-6s  ✓\n" "$name" "$gpg_count" "$deb_count" "$size"
    else
        printf "%-30s  GPG:%-2d  DEB:%-2d  SIZE:%-6s  ✗\n" "$name" "$gpg_count" "$deb_count" "$size"
    fi
done | sort -t: -k2 -rn
