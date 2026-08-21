ARG REGISTRY
ARG IMAGE_TAG=latest
FROM ${REGISTRY}/serverless-init:${IMAGE_TAG} AS serverless-init

FROM python:3.11-slim

COPY --from=serverless-init /serverless-init /serverless-init

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py job.py ./

EXPOSE 8080

# serverless-init wraps the app: starts the embedded DD agent, then exec's CMD.
# For services: CMD is overridden at deploy time with "python app.py".
# For jobs:     CMD is overridden at deploy time with "python job.py".
ENTRYPOINT ["/serverless-init"]
CMD ["python", "app.py"]
