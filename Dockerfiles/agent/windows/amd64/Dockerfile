ARG BASE_IMAGE=mcr.microsoft.com/powershell:lts-windowsservercore-1809
ARG WITH_JMX="false"
ARG VARIANT="unknown"
ARG INSTALL_INFO="unknown"

# Extract the embedded3.7z in a separate stage
FROM mcr.microsoft.com/powershell:lts-windowsservercore-${VARIANT} as unzipper
USER ContainerAdministrator
SHELL ["pwsh", "-Command", "$ErrorActionPreference = 'Stop';"]
COPY datadog-agent-latest.amd64.zip uncompress-zip.ps1 ./
RUN . ./uncompress-zip.ps1

# Build the containerized Agent
FROM ${BASE_IMAGE}

LABEL maintainer "Datadog <package@datadoghq.com>"

USER ContainerAdministrator

SHELL ["pwsh", "-Command", "$ErrorActionPreference = 'Stop';"]

COPY install.ps1 ./
COPY --from=unzipper ["C:/ProgramData/Datadog", "C:/ProgramData/Datadog"]
COPY --from=unzipper ["C:/Program Files/Datadog", "C:/Program Files/Datadog"]
RUN . ./install.ps1

EXPOSE 8125/udp 8126/tcp

COPY entrypoint.exe C:/entrypoint.exe
ADD entrypoint-ps1 ./entrypoint-ps1
COPY datadog*.yaml C:/ProgramData/Datadog/

ENTRYPOINT ["C:/entrypoint.exe"]
CMD ["datadogagent"]
