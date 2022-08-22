ROOT_DIR=/git/datadog-agent

if [[ -z $DD_SKIP_VENV ]]; then
DD_VENV_DIR=venv

source "$DD_VENV_DIR"/bin/activate
fi

pushd "$ROOT_DIR"
DELVE=1 invoke agent.build --build-exclude=systemd
popd
