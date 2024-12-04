#!/bin/bash
set -eEuxo pipefail

runner_config=$@
docker_dir=/kmt-dockers

# Add provisioning steps here !
## Start docker
systemctl start docker
## Load docker images
if [[ -d "${docker_dir}" ]]; then
  find "${docker_dir}" -maxdepth 1 -type f -exec docker load -i {} \;
fi
# VM provisioning end !

# Copy BTF files. This is a patch for different paths between agent 6 branch code and agent 7 KMT images
if [ -d "/system-probe-tests" ]; then
    rsync -avP /system-probe-tests /opt/kmt-ramfs
fi

# Start tests
code=0
/test-runner $runner_config || code=$?

if [[ -f "/job_env.txt" ]]; then
    cp /job_env.txt /ci-visibility/junit/
else
    echo "job_env.txt not found. Continuing without it."
fi

tar -C /ci-visibility/testjson -czvf /ci-visibility/testjson.tar.gz .
tar -C /ci-visibility/junit -czvf /ci-visibility/junit.tar.gz .

exit ${code}
