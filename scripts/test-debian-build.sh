#!/bin/bash

# Test script to validate Debian package setup
set -e

echo "Testing Debian package structure..."

# Check required files exist
files=(
    "debian/control"
    "debian/rules" 
    "debian/changelog"
    "debian/compat"
    "debian/postinst"
    "debian/prerm"
    "debian/accel-exporter.service"
)

for file in "${files[@]}"; do
    if [ ! -f "$file" ]; then
        echo "ERROR: Missing required file: $file"
        exit 1
    fi
    echo "✓ Found: $file"
done

# Test changelog template replacement
echo "Testing changelog template..."
TEST_VERSION="1.2.3"
TEST_DATE=$(date -R)

sed -e "s/__VERSION__/${TEST_VERSION}/g" \
    -e "s/__DATE__/${TEST_DATE}/" \
    debian/changelog > /tmp/test-changelog

if grep -q "${TEST_VERSION}-1" /tmp/test-changelog; then
    echo "✓ Changelog template replacement works"
else
    echo "ERROR: Changelog template replacement failed"
    exit 1
fi

# Check executable permissions
if [ -x "debian/rules" ]; then
    echo "✓ debian/rules is executable"
else
    echo "ERROR: debian/rules is not executable"
    exit 1
fi

if [ -x "debian/postinst" ]; then
    echo "✓ debian/postinst is executable"
else
    echo "ERROR: debian/postinst is not executable"
    exit 1
fi

if [ -x "debian/prerm" ]; then
    echo "✓ debian/prerm is executable"
else
    echo "ERROR: debian/prerm is not executable"
    exit 1
fi

echo "✅ All Debian package structure tests passed!"
echo ""
echo "Package will create:"
echo "  - Binary: /usr/bin/accel-exporter"
echo "  - Service: /lib/systemd/system/accel-exporter.service"
echo "  - User: accel-exporter"
echo "  - Data dir: /var/lib/accel-exporter"

rm -f /tmp/test-changelog