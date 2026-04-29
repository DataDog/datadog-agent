FROM ubuntu:latest AS src

COPY . /usr/src/datadog-agent

RUN find /usr/src/datadog-agent -type f \! -name \*.go -print0 | xargs -0 rm
RUN find /usr/src/datadog-agent -type d -empty -print0 | xargs -0 rmdir

FROM ubuntu:latest AS bin

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get clean &&     apt-get -o Acquire::Retries=4 update &&     apt-get install -y patchelf

COPY bin/agent/agent                            /opt/datadog-agent/bin/agent/agent
COPY bin/agent/dist/conf.d                      /etc/datadog-agent/conf.d
COPY dev/lib/libdatadog-agent-rtloader.so.0.1.0 /opt/datadog-agent/embedded/lib/libdatadog-agent-rtloader.so.0.1.0
COPY dev/lib/libdatadog-agent-three.so          /opt/datadog-agent/embedded/lib/libdatadog-agent-three.so

COPY dev/lib/libpatterns.so /opt/datadog-agent/embedded/lib/libpatterns.so

RUN patchelf --set-rpath /opt/datadog-agent/embedded/lib /opt/datadog-agent/bin/agent/agent
RUN patchelf --set-rpath /opt/datadog-agent/embedded/lib /opt/datadog-agent/embedded/lib/libdatadog-agent-rtloader.so.0.1.0
RUN patchelf --set-rpath /opt/datadog-agent/embedded/lib /opt/datadog-agent/embedded/lib/libdatadog-agent-three.so
RUN patchelf --set-rpath /opt/datadog-agent/embedded/lib /opt/datadog-agent/embedded/lib/libpatterns.so


FROM golang:latest AS dlv

RUN go install github.com/go-delve/delve/cmd/dlv@latest

FROM gcr.io/datadoghq/agent:7.78.2 AS bash_completion

RUN apt-get clean &&     apt-get -o Acquire::Retries=4 update &&     apt-get install -y gawk

RUN awk -i inplace '!/^#/ {uncomment=0} uncomment {gsub(/^#/, "")} /# enable bash completion/ {uncomment=1} {print}' /etc/bash.bashrc

FROM gcr.io/datadoghq/agent:7.78.2

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get clean &&     apt-get -o Acquire::Retries=4 update &&     apt-get install -y bash-completion less vim tshark &&     apt-get clean

ENV DELVE_PAGER=less

COPY --from=dlv /go/bin/dlv /usr/local/bin/dlv
COPY --from=bash_completion /etc/bash.bashrc /etc/bash.bashrc
COPY --from=src /usr/src/datadog-agent /root/repos/datadog-agent
COPY --from=bin /opt/datadog-agent/bin/agent/agent                                 /opt/datadog-agent/bin/agent/agent
COPY --from=bin /opt/datadog-agent/embedded/lib/libdatadog-agent-rtloader.so.0.1.0 /opt/datadog-agent/embedded/lib/libdatadog-agent-rtloader.so.0.1.0
COPY --from=bin /opt/datadog-agent/embedded/lib/libdatadog-agent-three.so          /opt/datadog-agent/embedded/lib/libdatadog-agent-three.so
COPY --from=bin /opt/datadog-agent/embedded/lib/libpatterns.so /opt/datadog-agent/embedded/lib/libpatterns.so
COPY --from=bin /etc/datadog-agent/conf.d /etc/datadog-agent/conf.d


RUN agent          completion bash > /usr/share/bash-completion/completions/agent
RUN process-agent  completion bash > /usr/share/bash-completion/completions/process-agent
RUN security-agent completion bash > /usr/share/bash-completion/completions/security-agent
RUN system-probe   completion bash > /usr/share/bash-completion/completions/system-probe
RUN trace-agent    completion bash > /usr/share/bash-completion/completions/trace-agent

ENV DD_SSLKEYLOGFILE=/tmp/sslkeylog.txt
