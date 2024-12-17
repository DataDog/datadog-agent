# from https://github.com/open-telemetry/opentelemetry-collector-releases/blob/main/distributions/otelcol-contrib/Dockerfile
FROM otel/opentelemetry-collector-contrib:latest AS prep
from ubuntu:latest

COPY --from=prep /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=prep otelcol-contrib /otelcol-contrib
COPY --from=prep /etc/otelcol-contrib /etc/otelcol-contrib


EXPOSE 4317 55680 55679


COPY minimal.yml /etc/otelcol-contrib/minimal.yaml
COPY entrypoint.sh /bin/entrypoint.sh

ENTRYPOINT ["/bin/entrypoint.sh"]

