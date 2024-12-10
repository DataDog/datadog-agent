FROM alpine:latest AS builder

# Install a statically linked shell and the necessary binaries
RUN apk add --no-cache busybox-static
RUN apk add --no-cache coreutils acl attr

# from https://github.com/open-telemetry/opentelemetry-collector-releases/blob/main/distributions/otelcol-contrib/Dockerfile
FROM otel/opentelemetry-collector-contrib:latest AS prep

FROM scratch

# Copy the shell executable from the builder stage
COPY --from=builder /bin/busybox.static /bin/sh
# Copy id binary from the builder stage
COPY --from=builder /usr/bin/id /bin/id

# Copy required shared libraries
COPY --from=builder /lib/ld-musl-x86_64.so.* /lib/
COPY --from=builder /lib/libc.musl-x86_64.so.* /lib/
COPY --from=builder /lib/libcrypto.so.* /lib/
COPY --from=builder /lib/libacl.so.* /lib/
COPY --from=builder /lib/libattr.so.* /lib/
COPY --from=builder /lib/libutmps.so.* /lib/
COPY --from=builder /lib/libskarnet.so.* /lib/
COPY --from=builder /lib/libutmps.so.* /lib/

COPY --from=prep /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=prep otelcol-contrib /otelcol-contrib

ARG USER_GID=10001
USER ${USER_UID}:${USER_GID}



EXPOSE 4317 55680 55679


COPY minimal.yml /etc/otelcol-contrib/config.yaml
COPY entrypoint.sh /bin/entrypoint.sh

ENTRYPOINT ["/bin/entrypoint.sh"]
