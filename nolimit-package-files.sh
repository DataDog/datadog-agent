#!/bin/bash

die() {
    echo "Error: $1, exiting"
    exit 0
}

echo "# compiling system-probe"
inv -e system-probe.build --static --bundle-ebpf --no-bundle || die "compilation failed"

echo "# creating the final 'package'"
tar cvzf system-probe-nolimit.tgz system-probe.sh -C ./bin/system-probe/ system-probe

echo ""
echo "Package system-probe-nolimit.tgz has been created. To use, copy/untar it somewhere, then run ./system-probe.sh"
