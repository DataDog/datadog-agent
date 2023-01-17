ARG BASE_IMAGE=mcr.microsoft.com/powershell:lts-windowsservercore-1809
FROM ${BASE_IMAGE}
LABEL baseimage.os "windows server core"
LABEL baseimage.name "${BASE_IMAGE}"

ARG WITH_JMX="false"
ARG VARIANT="unknown"
ARG INSTALL_INFO="unknown"
ARG GENERAL_ARTIFACTS_CACHE_BUCKET_URL

LABEL maintainer "Datadog <package@datadoghq.com>"

USER ContainerAdministrator

SHELL ["pwsh", "-Command", "$ErrorActionPreference = 'Stop';"]

COPY datadog-agent-latest.amd64.zip install.ps1 ./
RUN . ./install.ps1

EXPOSE 8125/udp 8126/tcp

COPY entrypoint.exe C:/entrypoint.exe
ADD entrypoint-ps1 ./entrypoint-ps1
COPY datadog*.yaml C:/ProgramData/Datadog/

ENTRYPOINT ["C:/entrypoint.exe"]
CMD ["datadogagent"]
