FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
  && apt-get install -y --no-install-recommends \
    bpftrace \
    ca-certificates \
  && rm -rf /var/lib/apt/lists/*

COPY trace-arm64-singlestep.bt /usr/local/share/datadog/trace-arm64-singlestep.bt

ENTRYPOINT ["bpftrace", "/usr/local/share/datadog/trace-arm64-singlestep.bt"]
