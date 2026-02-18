#!/bin/bash

set -e

# Try to disable unattended upgrades and apt automatic updates, should not fail if it is not installed
sudo systemctl disable unattended-upgrades.service || true
sudo systemctl stop unattended-upgrades.service || true

sudo systemctl disable apt-daily.service || true
sudo systemctl disable apt-daily.timer || true
sudo systemctl stop apt-daily.service || true
sudo systemctl stop apt-daily.timer || true

sudo systemctl disable apt-daily-upgrade.service || true
sudo systemctl disable apt-daily-upgrade.timer || true
sudo systemctl stop apt-daily-upgrade.service || true
sudo systemctl stop apt-daily-upgrade.timer || true

# Send the TERM signal to any remaining unattended-upgrades processes
pkill --signal TERM -f unattended-upgrade || true

max_to_wait=10
while pgrep -f unattended-upgrade && [ $max_to_wait -gt 0 ]; do
	echo "Waiting for unattended-upgrade to terminate"
	sleep 1
	max_to_wait=$((max_to_wait - 1))
done

# Kill any unattended-upgrade processes that didn't terminate
pkill --signal KILL -f unattended-upgrade || true

# Ensure the lock files are removed
rm -f /var/lib/apt/lists/lock || true
rm -f /var/cache/apt/archives/lock || true
rm -f /var/lib/dpkg/lock || true
rm -f /var/lib/dpkg/lock-frontend || true
rm -f /var/cache/apt/archives/partial/lock || true

# Important note: we're searching for unattended-upgrade for the process because
# apparently there can be 'unattended-upgrades' and 'unattended-upgrade' commands
# However, the APT package is called unattended-upgrades

apt-get -y purge unattended-upgrades || true

# Ensure the lock files are removed
rm -f /var/lib/apt/lists/lock || true
rm -f /var/cache/apt/archives/lock || true
rm -f /var/lib/dpkg/lock || true
rm -f /var/lib/dpkg/lock-frontend || true
rm -f /var/cache/apt/archives/partial/lock || true
