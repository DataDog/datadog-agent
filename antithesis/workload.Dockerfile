# The workload previously bind-mounted setup-complete.sh and test/ from the
# host filesystem (`../setup-complete.sh:/antithesis/setup-complete.sh:ro`).
# That works locally because those paths exist on the host, but Antithesis's
# real environment only ships whatever snouty packages into the config image
# from antithesis/config/ — files one directory up don't exist there, so the
# bind mount silently created an empty directory instead
# (`/antithesis/setup-complete.sh: Is a directory`), which is why the
# workload's setup-complete.sh invocation failed on every real run. Bake it
# into the image at build time instead, like every other service here.
FROM debian:stable-slim
COPY antithesis/setup-complete.sh /antithesis/setup-complete.sh
RUN chmod +x /antithesis/setup-complete.sh && mkdir -p /opt/antithesis/test/v1
