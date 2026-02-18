FROM datadog/agent:7.67.0-rc.6-full
# Build from repo root: docker build -f test/minikube-cluster-agent/Dockerfile -t zork:latest .
COPY test/minikube-cluster-agent/datadog.yaml /etc/datadog-agent/datadog-cluster.yaml
COPY bin/datadog-cluster-agent/datadog-cluster-agent /opt/datadog-agent/bin/agent/datadog-cluster-agent
# Ship secret-generic-connector with the cluster agent (750 = no others, no group write)
COPY bin/secret-generic-connector/secret-generic-connector /opt/datadog-agent/bin/secret-generic-connector
RUN chmod 750 /opt/datadog-agent/bin/secret-generic-connector
