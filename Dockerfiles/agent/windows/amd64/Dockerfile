ARG BASE_IMAGE=mcr.microsoft.com/powershell:lts-windowsservercore-1809
ARG NETAPI32_IMAGE=mcr.microsoft.com/powershell:lts-windowsservercore-1809

FROM ${NETAPI32_IMAGE} AS netapi32

FROM ${BASE_IMAGE} AS final
# This "copy netapi32.dll" hack can be reverted once https://github.com/microsoft/Windows-Containers/issues/72 is fixed.
COPY --from=netapi32 /Windows/System32/netapi32.dll /netapi32.dll

ARG WITH_JMX="false"
ARG VARIANT="unknown"

LABEL maintainer "Datadog <package@datadoghq.com>"

USER ContainerAdministrator

SHELL ["pwsh", "-Command", "$ErrorActionPreference = 'Stop';"]

RUN if (-not (Test-Path /Windows/System32/netapi32.dll)) { \
        Move-Item /netapi32.dll -Destination /Windows/System32/netapi32.dll \
    } else { \
        Remove-Item /netapi32.dll \
    }

COPY datadog-agent-latest.amd64.zip install.ps1 ./
RUN . ./install.ps1

EXPOSE 8125/udp 8126/tcp

COPY entrypoint.exe C:/entrypoint.exe
ADD entrypoint-ps1 ./entrypoint-ps1
COPY datadog*.yaml C:/ProgramData/Datadog/

ENTRYPOINT ["C:/entrypoint.exe"]
CMD ["datadogagent"]
