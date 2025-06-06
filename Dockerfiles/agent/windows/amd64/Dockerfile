ARG BASE_IMAGE=mcr.microsoft.com/powershell:lts-windowsservercore-1809
FROM ${BASE_IMAGE}
ARG BASE_IMAGE
LABEL baseimage.os "windows server core"
LABEL baseimage.name "${BASE_IMAGE}"
LABEL org.opencontainers.image.base.name "${BASE_IMAGE}"
LABEL org.opencontainers.image.title="Datadog Agent"

ARG WITH_JMX="false"
ARG VARIANT="unknown"
ARG INSTALL_INFO="unknown"
ARG GENERAL_ARTIFACTS_CACHE_BUCKET_URL
# Should be empty for non-FIPS builds
ARG WITH_FIPS

LABEL maintainer "Datadog <package@datadoghq.com>"
LABEL org.opencontainers.image.authors="Datadog <package@datadoghq.com>"
LABEL org.opencontainers.image.source="https://github.com/DataDog/datadog-agent"
LABEL org.opencontainers.image.vendor="Datadog, Inc."

USER ContainerAdministrator

SHELL ["pwsh", "-Command", "$ErrorActionPreference = 'Stop';"]

COPY ["Datadog Agent", "C:/Program Files/Datadog/Datadog Agent"]

COPY install.ps1 ./
RUN . ./install.ps1

COPY bouncycastle-fips /fips-build
COPY install-fips.ps1 ./
RUN . ./install-fips.ps1
ENV JAVA_TOOL_OPTIONS="${WITH_FIPS:+--module-path=\'c:\\program files\\datadog\\bouncycastle fips\' -Djava.security.properties==\'c:\\program files\\datadog\\bouncycastle fips\\java.security\' -Dpolicy.url.2=\'file:c:\\program files\\datadog\\bouncycastle fips\\bc-fips.policy\'}"

EXPOSE 8125/udp 8126/tcp

COPY entrypoint.exe C:/entrypoint.exe
ADD entrypoint-ps1 ./entrypoint-ps1
COPY datadog*.yaml C:/ProgramData/Datadog/

ENTRYPOINT ["C:/entrypoint.exe"]
CMD ["datadogagent"]
