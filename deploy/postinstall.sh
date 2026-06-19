#!/bin/sh
# Run after the package is installed or upgraded. Creates the service user and
# data directory, then enables the unit. Mirrors the old debian/postinst.
set -e

if ! getent passwd accel-exporter >/dev/null 2>&1; then
	adduser --system --group --home /var/lib/accel-exporter \
		--no-create-home --shell /bin/false \
		--gecos "Accel-PPP Exporter" accel-exporter
fi

mkdir -p /var/lib/accel-exporter
chown accel-exporter:accel-exporter /var/lib/accel-exporter

if [ -d /run/systemd/system ]; then
	systemctl daemon-reload
	systemctl enable accel-exporter.service
fi
