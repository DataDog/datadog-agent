ARG BASE_IMAGE=servercore:1803

FROM mcr.microsoft.com/windows/${BASE_IMAGE}

LABEL maintainer "Datadog <package@datadoghq.com>"

COPY datadog-agent-7-latest.amd64.msi .
RUN msiexec /i datadog-agent-7-latest.amd64.msi /qn

EXPOSE 8125/udp 8126/tcp

# Config file is choosen at runtime by the entrypoint script
RUN del C:/ProgramData/Datadog/datadog.yaml
COPY entrypoint.ps1 .
ADD entrypoint-ps1 ./entrypoint-ps1
COPY datadog*.yaml C:/ProgramData/Datadog

ENTRYPOINT ["powershell", "./entrypoint.ps1"]

CMD ["run"]
