# The official registry.k8s.io/kube-apiserver image is distroless (no shell, no
# wget/curl) so Docker Compose healthchecks cannot exec anything inside it. This
# repackages the same binary onto Alpine (whose busybox wget is linked against a
# real TLS backend, unlike plain busybox:1.36) so a healthcheck can run.
FROM registry.k8s.io/kube-apiserver:v1.31.1 AS kubebin

FROM alpine:3.20
COPY --from=kubebin /usr/local/bin/kube-apiserver /usr/local/bin/kube-apiserver
ENTRYPOINT ["/usr/local/bin/kube-apiserver"]
