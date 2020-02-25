ARG BASE_IMAGE=servercore:ltsc2019
FROM mcr.microsoft.com/windows/${BASE_IMAGE}
ARG MSI_FILE=datadog-agent-7-latest.amd64.msi
LABEL maintainer "Datadog <package@datadoghq.com>"
WORKDIR C:\\mnt
COPY ${MSI_FILE} .
EXPOSE 8125/udp 8126/tcp
RUN msiexec /i C:\mnt\%MSI_FILE% /qn
RUN del /F /Q C:\mnt
ENTRYPOINT ["C:/Program Files/Datadog/Datadog Agent/bin/agent.exe"]
CMD ["run"]
