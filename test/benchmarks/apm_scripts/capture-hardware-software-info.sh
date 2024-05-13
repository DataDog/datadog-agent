#!/usr/bin/env bash

cd "${ARTIFACTS_DIR}"

# Collect software information

(which top && top -b -n 1 > top.txt) || :
(which uname && uname -a > uname.txt) || :
(which ldconfig && ldconfig -v > ldconfig.txt) || :
(which sysctl && sysctl -a > sysctl.txt) || :

# Collect hardware information

(which lscpu && lscpu -e > lscpu-e.txt) || :
(which lscpu && lscpu > lscpu.txt) || :
(which hwinfo && hwinfo --short > hwinfo-short.txt) || :
(which hwinfo && hwinfo > hwinfo-full.txt) || :
#cpupower frequency-info > cpupower-frequency-info.txt
#turbostat -n 1 > turbostat.txt
