ARG BASE_IMAGE=mcr.microsoft.com/powershell:lts-windowsservercore-1809
ARG DATADOG_AGENT_WINBUILDIMAGES
FROM 486234852809.dkr.ecr.us-east-1.amazonaws.com/ci/datadog-agent-buildimages/windows_1809_x64:${DATADOG_AGENT_WINBUILDIMAGES}
ADD windows/entrypoint c:/entrypoint
WORKDIR c:/entrypoint
RUN ["C:\\Program Files (x86)\\Microsoft Visual Studio\\2017\\BuildTools\\VC\\Auxiliary\\Build\\vcvars64.bat", "&&", "msbuild", "/p:Configuration=Release", "/p:Platform=x64"]

FROM ${BASE_IMAGE}

ARG WITH_JMX="false"

LABEL maintainer "Datadog <package@datadoghq.com>"

USER ContainerAdministrator

SHELL ["pwsh", "-Command", "$ErrorActionPreference = 'Stop';"]

COPY datadog-agent-7-latest.amd64.zip install.ps1 ./
RUN . ./install.ps1

EXPOSE 8125/udp 8126/tcp

# Config file is choosen at runtime by the entrypoint script
COPY --from=0 C:\entrypoint\x64\Release\entrypoint.exe C:\entrypoint.exe
ADD entrypoint-ps1 ./entrypoint-ps1
COPY datadog*.yaml C:/ProgramData/Datadog/
COPY install_info C:/ProgramData/Datadog/

ENTRYPOINT ["C:/entrypoint.exe"]
CMD ["agent", "run"]
