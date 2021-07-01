FROM python:3.9-alpine

COPY app /opt/fake_datadog

RUN apk update && \
    apk add python3-dev gcc g++ musl-dev libstdc++ && \
    pip install -r /opt/fake_datadog/requirements.txt && \
    apk del python3-dev gcc g++ musl-dev && \
    rm -rf /var/cache/apk/*

VOLUME /opt/fake_datadog/recorded

ENV prometheus_multiproc_dir "/var/lib/prometheus"

CMD ["gunicorn", "--bind", "0.0.0.0:80", "--pythonpath", "/opt/fake_datadog", "api:app"]
