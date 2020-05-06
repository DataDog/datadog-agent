cp datadog.yaml bin/agent/dist/datadog.yaml

cp kafka-jmx.conf.yaml bin/agent/dist/conf.d/jmx.d/conf.yaml

mkdir -p bin/agent/dist/jmx/
cp jmxfetch-0.36.1-jar-with-dependencies.jar bin/agent/dist/jmx/jmxfetch.jar

sleep 1

sudo ./bin/agent/agent -c bin/agent/dist/datadog.yaml check jmx --log-level trace --json
