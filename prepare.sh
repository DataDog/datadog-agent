#!/bin/bash

die() {
    echo "Error: $1, exiting"
    exit 0
}

PROBES_INSTALL_PATH=/opt/datadog-agent/embedded/share/system-probe/ebpf
RUNTIME_PATH=/opt/datadog-agent/run

sudo mkdir -p $RUNTIME_PATH || die "mkdir $RUNTIME_PATH"
sudo mkdir -p $PROBES_INSTALL_PATH || die "mkdir $PROBES_INSTALL_PATH"
sudo tar xvf probes.tgz -C $PROBES_INSTALL_PATH || die "untar probes"
sudo chown -R root:root $PROBES_INSTALL_PATH || die "chown probes"
sudo chmod 644 $PROBES_INSTALL_PATH/*o || die "chmod probes"

echo ""
echo "Probes has been copied, you can run system-probe and wait for events in /tmp/dd_events"
echo "(set STREAM_OUTPUT_DIR to specify another output directory)"
