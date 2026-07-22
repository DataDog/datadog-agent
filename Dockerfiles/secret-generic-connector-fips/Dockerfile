# FIPS secret-generic-connector image.
# The FIPS binary is built with CGO (boringcrypto), so it is dynamically linked and needs glibc
# at runtime — a scratch base is not enough. Use the FIPS-blessed distroless base, which ships
# glibc (it already runs the boringcrypto operator/agent binaries) and no OpenSSL (boringcrypto
# bundles its own crypto module).
# hadolint ignore=DL3026
FROM registry.ddbuild.io/images/base/gbi-distroless-nossl-fips:release
ARG TARGETARCH
COPY --chmod=0755 ./secret-generic-connector.$TARGETARCH /secret-generic-connector
USER 1000
