FROM scratch
ARG TARGETARCH
COPY --chmod=0755 ./datadog-cws-instrumentation/cws-instrumentation.$TARGETARCH /cws-instrumentation
USER 1000
