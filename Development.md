# Compiling the StackState agent using Docker

Run the stackstate agent runner gitlab docker container:
```bash
docker run -ti docker.io/stackstate/stackstate-agent-runner-gitlab:deb_20190429
```

inside the container execute the following command:
```bash
export CI_PROJECT_DIR=/go/src/github.com/StackVista/stackstate-agent && \
mkdir -p /go/src/github.com/StackVista && \
cd src/github.com/StackVista && \
git clone https://github.com/StackVista/stackstate-agent && \
cd stackstate-agent && \
git checkout ${STACKSTATE_AGENT_BRANCH_NAME} && \
source .gitlab-scripts/setup_env.sh && \
inv -e agent.omnibus-build --base-dir /omnibus --skip-sign
```

mounting your local directory into the gitlab docker container: 
```bash
docker run -it --name stackstate-agent-builder --mount type=bind,source="${PWD}",target=/go/src/github.com/StackVista/stackstate-agent,readonly docker.io/stackstate/stackstate-agent-runner-gitlab:deb_20190429
```
