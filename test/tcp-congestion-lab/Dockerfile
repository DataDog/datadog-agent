FROM ubuntu:22.04
RUN apt-get update && apt-get install -y --no-install-recommends \
    iperf3 iproute2 iptables iputils-ping python3 curl netcat-openbsd \
    && rm -rf /var/lib/apt/lists/*
