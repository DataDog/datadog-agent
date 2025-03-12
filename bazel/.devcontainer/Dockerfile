FROM ubuntu:22.04

# Install essential dependencies
RUN apt-get update -y \
    && apt-get install -y \
    curl git build-essential

# Install bazelisk
RUN curl -LO https://github.com/bazelbuild/bazelisk/releases/download/v1.20.0/bazelisk-linux-arm64 \
    && chmod +x /bazelisk-linux-arm64 \
    && ln -s /bazelisk-linux-arm64 /usr/local/bin/bazel

# Run forever so that we can interact with the container via `docker exec`
ENTRYPOINT ["tail", "-f", "/dev/null"]
