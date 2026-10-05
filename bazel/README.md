As you may have noticed, we've been transitioning to the `bazel` build system.

Although most elements are still under development, here are a few notes that could "help us help you."

### Single requirement: `bazelisk`

> [!TIP]
> We recommend using Bazelisk through `dda`, our developer tool.
>
> 1. Ensure that the latest version is [installed](https://datadoghq.dev/datadog-agent/setup/required/#tooling).
> 2. Use the [`dda bzl`](https://datadoghq.dev/datadog-agent-dev/reference/cli/commands/#dda-bzl) Bazel wrapper in place of all `bazel` commands. This will forward all arguments to Bazel and transparently download Bazelisk if there is no `bazel` nor `bazelisk` in your PATH.

If your OS or dev container does not already provide it, you will need to install the `bazelisk` tool, which will
automatically switch to the version of `bazel` specified in the branch you wish to contribute to.

We recommend using `brew` because the package also installs a symbolic link named `bazel`:
(which is very useful on a daily basis, such as matching examples in the literature)

```sh
brew install bazelisk
```

Otherwise, please choose the `bazelisk` installation method that suits you best; you can find some of them here:

- [Installation](https://github.com/bazelbuild/bazelisk#installation)
- [Requirements](https://github.com/bazelbuild/bazelisk#requirements)

In that case, please consider adding a link to `bazelisk` named `bazel` in your PATH.

### Autocorrection of `bazel` files: `buildifier`

To help us maintain good `bazel` file hygiene, please preferably run the version of `buildifier` specified in the branch
you wish to work in:

```sh
dda bzl run //bazel/buildifier
# or
bazel run //bazel/buildifier
```

### Lock file maintenance

`MODULE.bazel.lock` must exhaustively reflect actually used dependencies. After updating any `bazel` dependency, such as
with `bazel_dep` in a `MODULE.bazel` file, please run:

```sh
bazel mod deps
```

### Remote cache (internal to Datadog)

The `tools/bazel` wrapper auto-selects the Buildbarn remote cache on local
builds. Behavior is controlled by `DD_BAZEL_REMOTE_CACHE`:

- `auto` (default): enable only when the frontend is reachable and a token
  source exists (the Vault CLI on the host, or an injected `BUILDBARN_ID_TOKEN`).
  Off-network contributors get a local build with no extra prompts.
- `on`: always enable; a failing credential helper aborts the build.
- `off`: never enable (disk cache stays active). Equivalent to passing
  `--config=no-remote-cache`.

To opt out persistently, either set `DD_BAZEL_REMOTE_CACHE=off` or add
`common --config=no-remote-cache` to `user.bazelrc` (workspace root) or your
`~/.bazelrc`: the wrapper detects an rc-level opt-out and skips injecting the
cache config. An explicit `--config=cache` / `--config=no-remote-cache` on the
command line always wins over auto-selection.

#### Remote cache in containers

Inside a container the credential helper cannot run an interactive Vault login
(it needs a browser), so auto-selection stays off until a token is injected via
the `BUILDBARN_ID_TOKEN` environment variable. Log in to Vault on the host (if
you have not already), mint the token there, and pass it in:

```sh
export VAULT_ADDR=https://vault.us1.ddbuild.io
export BUILDBARN_ID_TOKEN="$(
  vault read -address="$VAULT_ADDR" -field=token identity/oidc/token/buildbarn \
  || { vault login -address="$VAULT_ADDR" -method=oidc \
       && vault read -address="$VAULT_ADDR" -field=token identity/oidc/token/buildbarn; }
)"
docker run --env BUILDBARN_ID_TOKEN ...          # or: docker exec -e BUILDBARN_ID_TOKEN -it <container> ...
```

Running Bazel once on the host also performs the Vault OIDC login via the
credential helper, after which the `vault read` above succeeds without an extra
login step.

The OIDC token TTL is ~1h; re-mint it for long-lived shells.
