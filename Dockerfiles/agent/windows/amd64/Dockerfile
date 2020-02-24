FROM mcr.microsoft.com/windows/servercore:ltsc2019
LABEL maintainer "Datadog <package@datadoghq.com>"
WORKDIR C:\\mnt
COPY C:/datadog-agent-7.18.0-devel.git.82.99b5ce2-1-x86_64.msi .
EXPOSE 8125/udp 8126/tcp
RUN msiexec /qn /i C:\\mnt\\datadog-agent-7.18.0-devel.git.82.99b5ce2-1-x86_64.msi
CMD [ "START", "/W", "C:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe", "start-service" ]
