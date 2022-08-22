# This is the root directory of datadog agent project as set in the vagrant script
ROOT_DIR=/git/datadog-agent
cd $ROOT_DIR || exit

echo "Running the agent using dlv"
sudo dlv --listen=0.0.0.0:2345 --headless=true --api-version=2 --check-go-version=false --only-same-user=false exec ./bin/agent/agent -- run -c ./bin/agent/dist/datadog.yaml
exit
