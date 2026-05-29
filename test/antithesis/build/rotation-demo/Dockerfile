# Runtime image for the logs rotation-loss Antithesis demonstration.
#
# Build context is the antithesis/ directory (see docker-compose.yaml) so the image
# can include both the prebuilt demo binary and the test template.
#
# The instrumented (SDK-linked) binary is built on the host by build.sh and staged at
# build/rotation-demo/rotation-demo.bin. We copy a prebuilt binary (rather than
# rebuilding the multi-tool agent in-container) because the demo imports only
# pkg/logs, which compiles with a plain `go build`. The binary is dynamically linked
# against glibc, so the runtime base must provide glibc.
FROM debian:stable-slim

ENV NO_COLOR=1

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY build/rotation-demo/rotation-demo.bin /usr/local/bin/rotation-demo

# Antithesis test template (driver commands) discovered at /opt/antithesis/test/v1/.
COPY test/ /opt/antithesis/test/

ENTRYPOINT ["/usr/local/bin/rotation-demo"]
