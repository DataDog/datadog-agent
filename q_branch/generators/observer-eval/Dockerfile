FROM alpine/k8s:1.31.0

RUN apk add --no-cache bash jq gettext docker-cli docker-cli-buildx docker-cli-compose

COPY entrypoint.sh /entrypoint.sh
COPY agent-values.yaml.tmpl /templates/agent-values.yaml.tmpl
COPY episodes/ /episodes/

RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
