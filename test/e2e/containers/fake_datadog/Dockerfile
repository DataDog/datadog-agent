FROM python:3.6

COPY app /usr/local/fake_datadog/app

RUN pip install -r /usr/local/fake_datadog/app/requirements.txt && mkdir -pv /usr/local/fake_datadog/recorded

VOLUME /usr/local/fake_datadog/recorded

ENV prometheus_multiproc_dir "/var/lib/prometheus"

CMD ["gunicorn", "--bind", "0.0.0.0:80", "--pythonpath", "/usr/local/fake_datadog/app", "api:app"]
