# from https://github.com/open-telemetry/opentelemetry-collector-releases/blob/main/distributions/otelcol-contrib/Dockerfile
FROM otel/opentelemetry-collector-contrib:latest AS prep
FROM ubuntu:latest

COPY --from=prep /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=prep otelcol-contrib /otelcol-contrib
COPY --from=prep /etc/otelcol-contrib /etc/otelcol-contrib

RUN apt-get update && apt-get install -y curl lsof iftop nethogs
EXPOSE 4317 4318 55680 55679

COPY entrypoint.sh /bin/entrypoint.sh
ENTRYPOINT ["/bin/entrypoint.sh"]

