#!/usr/bin/env python3

import subprocess
import sys
import os.path

# Exclude some folders since go vet fails there
EXCLUDED_FOLDERS = {
    "./cmd/agent/android",
    "./cmd/agent/windows/service",
    "./cmd/cluster-agent",
    "./cmd/cluster-agent/app",
    "./cmd/system-probe",
    "./cmd/systray",
    "./pkg/clusteragent/orchestrator",
    "./pkg/process/config/testdata",
    "./pkg/process/util/orchestrator",
    "./pkg/trace/test/testsuite/testdata",
    "./pkg/util/containerd",
    "./pkg/util/containers/cri/crimock",
    "./pkg/util/containers/providers/cgroup",
    "./pkg/util/containers/providers/windows",
    "./pkg/util/hostname/apiserver",
    "./pkg/util/winutil",
    "./pkg/util/winutil/iphelper",
    "./pkg/util/winutil/pdhutil",
    "./test/benchmarks/aggregator",
    "./test/benchmarks/dogstatsd",
    "./test/integration/util/kube_apiserver",
}

# Exclude non go files
# Get the package for each file
targets = {"./" + os.path.dirname(path) for path in sys.argv[1:] if path.endswith(".go")}

# Call invoke command
# We do this workaround since we can't do relative imports
cmd = "inv -e vet --targets='{}'".format(",".join(targets - EXCLUDED_FOLDERS))

try:
    subprocess.run(cmd, shell=True, check=True)
except subprocess.CalledProcessError:
    # Signal failure to pre-commit
    sys.exit(-1)
