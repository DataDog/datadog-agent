FROM python:3.6-alpine as builder

COPY app /usr/local/fake_datadog/app

RUN apk update
RUN apk add py-pip python3-dev py-virtualenv gcc musl-dev

RUN virtualenv /usr/local/fake_datadog/venv -p python3 --no-site-packages --system-site-packages
RUN /usr/local/fake_datadog/venv/bin/pip install -r /usr/local/fake_datadog/app/requirements.txt
RUN mkdir -pv /usr/local/fake_datadog/recorded

FROM python:3.6-alpine

COPY --from=builder /usr/local/fake_datadog /usr/local/fake_datadog

VOLUME /usr/local/fake_datadog/recorded

ENV prometheus_multiproc_dir "/var/lib/prometheus"

CMD ["/usr/local/fake_datadog/venv/bin/gunicorn", "--bind", "0.0.0.0:80", "--pythonpath", "/usr/local/fake_datadog/app", "api:app"]
