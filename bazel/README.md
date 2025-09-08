As you may have noticed, we've been transitioning to the `bazel` build system.

Although most elements are still under development, here are a few notes that could "help us help you."

### Single requirement: `bazelisk`

If your OS or dev container does not already provide it, you will need to install the `bazelisk` tool, which will
automatically switch to the version of `bazel` specified in the branch you wish to contribute to.

We recommend using `brew` because the package also installs a symbolic link named `bazel`:
(which is very useful on a daily basis, such as matching examples in the literature)

```sh
brew install bazelisk
```

Otherwise, please choose the `bazelisk` installation method that suits you best; you can find some of them here:

- [Installation](https://github.com/bazelbuild/bazelisk?tab=readme-ov-file#installation)
- [Requirements](https://github.com/bazelbuild/bazelisk?tab=readme-ov-file#requirements)

In that case, please consider adding a link to `bazelisk` named `bazel` in your PATH.

### Autocorrection of `bazel` files: `buildifier`

To help us maintain good `bazel` file hygiene, please preferably run the version of `buildifier` specified in the branch
you wish to work in:

```sh
bazel run //bazel/buildifier
```

### Remote cache (internal to Datadog)

If you are on the Datadog internal network and want to take advantage of the remote cache, simply add the following line
to a `user.bazelrc` file located at the root of the workspace, or to a `.bazelrc` file located in your home directory:

```
common --config=cache
```
