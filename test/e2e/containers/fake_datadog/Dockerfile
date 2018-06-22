FROM python:3.7-rc as builder

COPY app /usr/local/fake_datadog/app

RUN apt-get update
RUN apt-get install -y python-virtualenv
RUN virtualenv /usr/local/fake_datadog/venv -p python3 --no-site-packages --system-site-packages
RUN /usr/local/fake_datadog/venv/bin/pip install -r /usr/local/fake_datadog/app/requirements.txt

FROM python:3.7-rc-slim

COPY --from=builder /usr/local/fake_datadog /usr/local/fake_datadog
RUN mkdir -pv /usr/local/fake_datadog/recorded

VOLUME /usr/local/fake_datadog/recorded

ENV prometheus_multiproc_dir "/var/lib/prometheus"

CMD ["/usr/local/fake_datadog/venv/bin/gunicorn", "--bind", "0.0.0.0:80", "--pythonpath", "/usr/local/fake_datadog/app", "api:app"]
