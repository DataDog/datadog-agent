ARG BASE_IMAGE=servercore:ltsc2019

FROM mcr.microsoft.com/windows/${BASE_IMAGE}

LABEL maintainer "Datadog <package@datadoghq.com>"

COPY datadog-agent-7-latest.amd64.msi .
RUN msiexec /i datadog-agent-7-latest.amd64.msi /qn

EXPOSE 8125/udp 8126/tcp

ENTRYPOINT ["C:/Program Files/Datadog/Datadog Agent/bin/agent.exe"]

CMD ["run"]
