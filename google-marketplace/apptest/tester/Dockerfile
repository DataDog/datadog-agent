FROM launcher.gcr.io/google/debian9

RUN apt-get update && apt-get install -y --no-install-recommends \
    bash \
    ca-certificates \
    gettext \
    jq \
    uuid-runtime \
    wget \
    && rm -rf /var/lib/apt/lists/*

RUN wget -q -O /bin/kubectl \
        https://storage.googleapis.com/kubernetes-release/release/v1.9.0/bin/linux/amd64/kubectl \
    && chmod 755 /bin/kubectl

COPY datadog-test-app.yaml.template /tester/
COPY tester.sh /tester/

WORKDIR /tester
ENTRYPOINT ["./tester.sh"]
