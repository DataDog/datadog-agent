FROM ubuntu:22.04
ARG USERNAME=bazel-user
ARG USER_UID=1000
ARG USER_GID=$USER_UID
# Create a non-root user
RUN groupadd --gid $USER_GID $USERNAME \
    && useradd --uid $USER_UID --gid $USER_GID -m $USERNAME
# Install essential dependencies
RUN apt-get update -y \
    && apt-get install -y \
    curl git build-essential

# Install bazelisk
RUN curl -LO https://github.com/bazelbuild/bazelisk/releases/download/v1.20.0/bazelisk-linux-arm64 \
    && chmod +x /bazelisk-linux-arm64 \
    && ln -s /bazelisk-linux-arm64 /usr/local/bin/bazel

RUN mkdir /tmp/bazel && chmod a+rwx /tmp/bazel

# Make Bazel output stuff to a volume we're mounting
RUN echo "startup --output_user_root=/tmp/bazel/build_output" > /etc/bazel.bazelrc

# Run forever so that we can interact with the container via `docker exec`
ENTRYPOINT ["tail", "-f", "/dev/null"]
