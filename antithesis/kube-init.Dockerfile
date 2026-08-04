# Antithesis runs are hermetic (no internet access once the run starts — see
# references/docker-compose.md "Hermetic Execution"). kube-init previously ran
# `apk add openssl` as its startup command, which needs network access and
# reliably fails under `snouty validate` / a real Antithesis run (it only worked
# under plain `docker compose up`, which still has normal internet access). Bake
# openssl in at BUILD time instead — building images happens before the run is
# sealed off.
FROM alpine:3.20
RUN apk add --no-cache openssl
