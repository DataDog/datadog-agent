FROM mcr.microsoft.com/windows/servercore:ltsc2019
ARG MSI_FILE=datadog-agent-7-latest.amd64.msi
LABEL maintainer "Datadog <package@datadoghq.com>"
WORKDIR C:\\mnt
COPY C:/${MSI_FILE}.msi .
EXPOSE 8125/udp 8126/tcp
RUN msiexec /qn /i C:\\mnt\\%MSI_FILE%.msi
ENTRYPOINT ["C:/Program Files/Datadog/Datadog Agent/bin/agent.exe"]
CMD ["run"]
