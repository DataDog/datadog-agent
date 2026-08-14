# Repackages the official registry.k8s.io/kube-apiserver image onto Alpine so
# Docker Compose healthchecks can exec inside it.
FROM registry.k8s.io/kube-apiserver:v1.31.1 AS kubebin

FROM alpine:3.20
COPY --from=kubebin /usr/local/bin/kube-apiserver /usr/local/bin/kube-apiserver
ENTRYPOINT ["/usr/local/bin/kube-apiserver"]
