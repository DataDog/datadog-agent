❌ ERROR: `bazelisk` is required to build this project.

To maximize reproducibility, `bazelisk` is the only supported entry point to bootstrap `bazel` and build the agent.

Actions:

1. [mandatory] ensure `bazelisk` is installed (see options below),
2. [recommended] create a symlink in your PATH named `bazel` pointing to `bazelisk`
   (so you can simply run `bazel` everywhere),
3. [recommended] uninstall any previously installed `bazel` binaries.

Quick install options (any reasonably recent version works):

- brew install bazelisk
- go install github.com/bazelbuild/bazelisk@latest
- npm install -g @bazel/bazelisk

For more install options, including native binaries:

- https://github.com/bazelbuild/bazelisk?tab=readme-ov-file#installation
- https://github.com/bazelbuild/bazelisk?tab=readme-ov-file#requirements
- https://github.com/bazelbuild/bazelisk/releases

💡 `bazelisk` is intended to become the only local/host dependency needed to build the agent.
