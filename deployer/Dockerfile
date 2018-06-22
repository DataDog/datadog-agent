FROM launcher.gcr.io/google/debian9 AS build

ADD schema.yaml /tmp/schema.yaml

ARG REGISTRY
ARG TAG
RUN cat /tmp/schema.yaml \
    | env -i "REGISTRY=$REGISTRY" "TAG=$TAG" envsubst \
    > /tmp/schema.yaml.new \
    && mv /tmp/schema.yaml.new /tmp/schema.yaml

FROM gcr.io/google-marketplace-tools/k8s/deployer_envsubst

COPY manifest /data/manifest
COPY test/deployer /data-test/
COPY --from=build /tmp/schema.yaml /data/
