FROM python:3.11-slim

COPY server.py __init__.py /app/
WORKDIR /app

ENV PORT=8082
ENV DD_SERVICE=python-svc-dd
ENV DD_VERSION=2.1
ENV DD_ENV=prod

CMD ["python3", "server.py"]
