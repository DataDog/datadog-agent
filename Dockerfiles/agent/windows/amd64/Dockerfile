ARG BASE_IMAGE=mcr.microsoft.com/powershell:nanoserver-20h2
FROM mcr.microsoft.com/powershell:lts-windowsservercore-1809 as unzipper
USER ContainerAdministrator
SHELL ["pwsh", "-Command", "$ErrorActionPreference = 'Stop';"]
COPY datadog-agent-latest.amd64.zip uncompress-zip.ps1 ./
RUN . ./uncompress-zip.ps1

FROM ${BASE_IMAGE}

ARG WITH_JMX="false"
ARG VARIANT="unknown"

LABEL maintainer "Datadog <package@datadoghq.com>"

USER ContainerAdministrator

SHELL ["pwsh", "-Command", "$ErrorActionPreference = 'Stop';"]

COPY --from=unzipper ["C:/ProgramData/Datadog", "C:/ProgramData/Datadog"]
COPY --from=unzipper ["C:/Program Files/Datadog", "C:/Program Files/Datadog"]
COPY install.ps1 ./
RUN . ./install.ps1

EXPOSE 8125/udp 8126/tcp

COPY entrypoint.exe C:/entrypoint.exe
ADD entrypoint-ps1 ./entrypoint-ps1
COPY datadog*.yaml C:/ProgramData/Datadog/

ENTRYPOINT ["C:/entrypoint.exe"]
CMD ["datadogagent"]
