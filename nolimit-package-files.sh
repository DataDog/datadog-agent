#!/bin/bash

die() {
    echo "Error: $1, exiting"
    exit 0
}

if [ ! -e ./bin/system-probe/system-probe ]; then
    echo "# compiling system-probe"
    inv -e system-probe.build --static --no-bundle || die "compilation failed"
fi

echo "# copying a tarball for eBPF programms"
tar cvzf probes.tgz -C pkg/ebpf/bytecode/build/x86_64/ . || die "probes tarball"

echo "# creating the final 'package'"
tar cvzf system-probe-nolimit.tgz prepare.sh probes.tgz -C ./bin/system-probe/ system-probe

rm probes.tgz || die "cleaning probes.tgz"
echo ""
echo "Package system-probe-nolimit.tgz has been created. To use, copy/untar it somewhere, run ./prepare.sh, then you can execute ./system-probe"
