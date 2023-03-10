FROM datadog/agent:latest

COPY agent/agent /opt/datadog-agent/bin/agent/agent
#COPY process-agent/process-agent /opt/datadog-agent/embedded/bin/process-agent