#!/bin/bash
set -eEuo pipefail

runner_config=$@
docker_dir=/kmt-dockers

# Add provisioning steps here !
## Start docker if available, some images (e.g. SUSE arm64 for CWS) do not have it installed
if command -v docker ; then
    systemctl start docker

    ## Load docker images
    if [[ -d "${docker_dir}" ]]; then
        find "${docker_dir}" -maxdepth 1 -type f -exec docker load -i {} \;
    fi
else
    echo "Docker not available, skipping docker provisioning"
fi
# VM provisioning end !

# Start tests
code=0

/opt/testing-tools/test-runner $runner_config || code=$?

if [[ -f "/job_env.txt" ]]; then
    cp /job_env.txt /ci-visibility/junit/
else
    echo "job_env.txt not found. Continuing without it."
fi

tar -C /ci-visibility/testjson -czvf /ci-visibility/testjson.tar.gz .
tar -C /ci-visibility/junit -czvf /ci-visibility/junit.tar.gz .

exit ${code}
