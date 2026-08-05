❌ ERROR: `bazelisk` is required to build this project.

A system-wide `bazel` binary is likely taking precedence over `bazelisk` and its `bazel` symlink.

`bazelisk` is the only supported entry point: it reads `.bazelversion` and bootstraps the required version of `bazel`,
whereas any other version is at best speculatively compatible.

To fix this, uninstall any system-wide Bazel for your platform:
- Ubuntu Linux: sudo apt purge bazel
- Windows: choco uninstall bazel / scoop uninstall bazel / winget uninstall bazel
- macOS: brew uninstall bazel

💡 Please run `inv install-tools`, which installs `bazelisk` and its `bazel` symlink alongside other Go binaries.
