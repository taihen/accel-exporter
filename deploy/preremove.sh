#!/bin/sh
# Run before the package is removed or upgraded. Stops and disables the unit so
# no orphaned process survives the uninstall. Mirrors the old debian/prerm.
set -e

if [ -d /run/systemd/system ]; then
	systemctl stop accel-exporter.service || true
	systemctl disable accel-exporter.service || true
fi
