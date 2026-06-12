FROM registry.ddbuild.io/ci/datadog-agent-buildimages/dev-env-workspace:v116983866-e2293bec

# Tools missing from the dev-env base image needed for local credential management.
# Most tools (gh, dd-gitsign, ddtool, gosu, dlv, etc.) are already in the base image.
RUN apt-get update && \
    apt-get install -y --no-install-recommends tmux pass && \
    rm -rf /var/lib/apt/lists/*

# Allow git to operate on the bind-mounted repo (different owner than container user).
RUN git config --system --add safe.directory /workspace

WORKDIR /workspace

# Override the default sshd CMD: the entrypoint creates the bits user from HOST_UID/HOST_GID,
# then execs into this shell instead of starting an SSH server.
CMD ["gosu", "bits", "bash"]
