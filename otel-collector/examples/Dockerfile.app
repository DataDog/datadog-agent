# Unless explicitly stated otherwise all files in this repository are dual-licensed
# under the Apache 2.0 or BSD3 Licenses.
FROM python:3



COPY ./otel-collector/examples/app /app
WORKDIR /app

RUN pip install -r requirements.txt
EXPOSE 8080
# Run the application with Datadog
CMD ["ddtrace-run", "python", "app.py"]

