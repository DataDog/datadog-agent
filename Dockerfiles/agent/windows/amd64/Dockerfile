ARG BASE_IMAGE=mcr.microsoft.com/windows/servercore:1809

# We will need some code changes to support nano. At least:
# - netapi32.dll is not there, so go's user.Current() doesn't work
# Also, "powershell" calls in this script have to be replaced by "pwsh"
#ARG BASE_IMAGE=mcr.microsoft.com/powershell:nanoserver-1809
FROM ${BASE_IMAGE} 

ARG WITH_JMX="false"

LABEL maintainer "Datadog <package@datadoghq.com>"

USER ContainerAdministrator

COPY datadog-agent-7-latest.amd64.zip install.ps1 ./
RUN powershell ./install.ps1

RUN setx PATH "%PATH%;C:\java\bin"
RUN if "%WITH_JMX%" == "false" (echo WITH_JMX not set, JRE not installed) else (java -version)

EXPOSE 8125/udp 8126/tcp

# Config file is choosen at runtime by the entrypoint script
COPY entrypoint.ps1 .
ADD entrypoint-ps1 ./entrypoint-ps1
COPY datadog*.yaml C:/ProgramData/Datadog/

ENTRYPOINT ["powershell", "./entrypoint.ps1"]

CMD ["run"]
