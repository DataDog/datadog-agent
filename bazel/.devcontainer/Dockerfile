ARG BASE_IMAGE=linux-glibc-2-17-x64
FROM registry.ddbuild.io/ci/datadog-agent-buildimages/${BASE_IMAGE}:v65307386-1b9b5175

RUN curl -LO https://github.com/bazelbuild/bazelisk/releases/download/v1.20.0/bazelisk-linux-$(dpkg --print-architecture) \
    && chmod +x /bazelisk-linux-$(dpkg --print-architecture) \
    && ln -s /bazelisk-linux-$(dpkg --print-architecture) /usr/local/bin/bazel

# Remove the build image cargo's config.toml as bazel will refuse to
# build if it's present since it would break hermeticity
RUN rm -f /.cargo/config.toml

# Make Bazel output stuff to a volume we're mounting
RUN echo "startup --output_user_root=/tmp/bazel/build_output" > /etc/bazel.bazelrc
# Add platform flag to bazelrc file to pick up the right glibc-targeting toolchain
ARG BASE_IMAGE
RUN \
  if [ "${BASE_IMAGE}" = "linux-glibc-2-17-x64" ]; then \
    echo "build --platforms=//bazel/platforms:linux_x64" > /etc/bazel.bazelrc; \
  else \
    echo "build --platforms=//bazel/platforms:linux_arm64" > /etc/bazel.bazelrc; \
fi
