FROM python:3.11-slim

RUN pip install ddtrace

COPY server.py __init__.py instrumented.py /app/
WORKDIR /app

ENV PORT=8083
ENV DD_SERVICE=python-instrumented-dd

CMD ["python3", "instrumented.py"]
