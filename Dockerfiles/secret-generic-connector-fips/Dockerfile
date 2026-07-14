# FIPS secret-generic-connector image.
#
# The FIPS binary is built with GOEXPERIMENT=boringcrypto + CGO_ENABLED=1, so its FIPS-validated
# crypto module (BoringSSL) is compiled INTO the binary. The base image therefore does not need a
# FIPS OpenSSL — it only needs glibc for the dynamically linked CGO binary (a scratch base, as used
# by the non-FIPS image, cannot run a dynamically linked binary). We use the same public,
# customer-reproducible Ubuntu base as the agent/cluster-agent images.
ARG BASE_IMAGE_UBUNTU_VERSION=24.04
ARG BASE_IMAGE_UBUNTU_NAME=noble
ARG BASE_IMAGE_REGISTRY=docker.io

FROM ${BASE_IMAGE_REGISTRY}/ubuntu:$BASE_IMAGE_UBUNTU_VERSION
ARG BASE_IMAGE_UBUNTU_VERSION
ARG BASE_IMAGE_UBUNTU_NAME
LABEL baseimage.os="ubuntu ${BASE_IMAGE_UBUNTU_NAME}"
LABEL baseimage.name="ubuntu:${BASE_IMAGE_UBUNTU_VERSION}"
LABEL org.opencontainers.image.base.name="ubuntu:${BASE_IMAGE_UBUNTU_VERSION}"
ARG TARGETARCH
COPY --chmod=0755 ./secret-generic-connector.$TARGETARCH /secret-generic-connector
USER 1000
