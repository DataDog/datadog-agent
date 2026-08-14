# Bind-mounts setup-complete.sh and test/ from the host filesystem.
FROM debian:stable-slim
COPY antithesis/setup-complete.sh /antithesis/setup-complete.sh
RUN chmod +x /antithesis/setup-complete.sh && mkdir -p /opt/antithesis/test/v1
