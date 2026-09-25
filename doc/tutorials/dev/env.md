# Using developer environments

-----

Developer environments are preconfigured workspaces that provide everything required for contributing to the Agent code base, testing changes and building release artifacts. Usually, these are [images](../../reference/images/dev.md) that run using a container orchestrator like Docker or Podman.

This tutorial will walk through how to use such environments with the [`dda env dev`](https://datadoghq.dev/datadog-agent-dev/reference/cli/commands/#dda-env-dev) command group.

/// note
This tutorial assumes you have already set up the [development requirements](../../setup/required.md).
///

## Overview

A developer environment may be used either as an ephemeral or persistent workspace. Multiple environments can be used concurrently, with each using a different checkout of the Agent repository.

To get an idea of upcoming interactions, run `dda env dev` and you should see the following output.

```
$ dda env dev

 Usage: dda env dev [OPTIONS] COMMAND [ARGS]...

╭─ Options ────────────────────────────────────────────────────────────╮
│ --help  -h  Show this message and exit.                              │
╰──────────────────────────────────────────────────────────────────────╯
╭─ Commands ───────────────────────────────────────────────────────────╮
│ cache   Manage the cache                                             │
│ code    Open a code editor for the developer environment             │
│ gui     Access a developer environment through a graphical interface │
│ remove  Remove a developer environment                               │
│ run     Run a command within a developer environment                 │
│ shell   Spawn a shell within a developer environment                 │
│ show    Show the available developer environments                    │
│ start   Start a developer environment                                │
│ status  Check the status of a developer environment                  │
│ stop    Stop a developer environment                                 │
╰──────────────────────────────────────────────────────────────────────╯
```

## Environment selection

When an environment is [started](#starting-environments), there are two options that permanently influence the environment until it is [removed](#removing-environments).

- The `--id` option sets the identifier that subsequent commands use to target the environment. All commands use the word `default` if no ID is provided. Environments of different types can have the same ID.
- The `-t`/`--type` option selects the type of developer environment. The following types are available:

    - `linux-container` - This runs the Linux developer environment and is the default type on non-Windows systems.

    More types will be added in the future. All non-macOS cloud environments will simply use the developer environment containers.

/// note
The default type on Windows is `windows-container` but that is not yet implemented. As a temporary workaround, use the `linux-container` type with the `-t`/`--type` start command option or [configure][dda-config] `dda` to use it as the [default type](https://datadoghq.dev/datadog-agent-dev/reference/api/config/#dda.config.model.env.DevEnvConfig.default_type) for developer environments with the following command.

```
dda config set env.dev.default-type linux-container
```
///

## Starting environments

You can start an environment with the `dda env dev start` command. All developer environment [types](#environment-selection) support the following options.

### Repository location

You may select where the code resides when starting an environment.

#### Local checkout

The default behavior assumes a local checkout of the [Agent repository](https://github.com/DataDog/datadog-agent). Clone the repo if you haven't already.

```
git clone https://github.com/DataDog/datadog-agent.git
```

Then enter the cloned directory and run the following command to start an environment.

```
dda env dev start
```

/// warning
There are two downsides to this approach.

1. It's not possible to have multiple environments using a separate checkout of the Agent repository i.e. you inherit the limitations of a usual Git checkout.
1. Bind mounts may experience performance issues, particularly for Git operations.
///

#### Remote clone

You can start an environment with a fresh clone of the Agent repository by passing the `--clone` flag.

```
dda env dev start --clone
```

You can also [configure][dda-config] `dda` to always [clone repositories](https://datadoghq.dev/datadog-agent-dev/reference/api/config/#dda.config.model.env.DevEnvConfig.clone_repos) for developer environments with the following command.

```
dda config set env.dev.clone-repos true
```

This will perform a shallow clone of the default branch, `main`.

### Repository selection

/// note
This functionality has no effect when using a [local checkout](#local-checkout).
///

The `-r`/`--repo` option selects the Datadog [repositories](../../reference/images/dev.md#repositories) to clone and may be supplied multiple times. By default, only the `datadog-agent` repository is chosen.

```
dda env dev start -r datadog-agent -r integrations-core
```

The first selected repository becomes the default location for the environment such as when [running commands](#running-commands) or [entering a shell](#entering-a-shell).

### Type-specific options

Each developer environment [type](#environment-selection) has additional options that are only available when using that type. For example, the `linux-container` type supports a `--no-pull` flag to disable the automatic pull of the latest image.

The help text displays options for the default type. In order to see options for a specific type, use the `-t`/`--type` option ***before*** the help flag like in the following example.

```
dda env dev start -t linux-container -h
```

## Environment status

You can check the status of an environment by running the following command.

```
dda env dev status
```

This shows the environment's state and any extra information that may be useful for debugging.

Environments have the following [possible states](https://datadoghq.dev/datadog-agent-dev/reference/interface/env/status/#dda.env.models.EnvironmentState).

- `started` - The environment is running.
- `stopped` - The environment is stopped.
- `starting` - The environment is starting.
- `stopping` - The environment is stopping.
- `error` - The environment is in an error state.
- `nonexistent` - The environment does not exist.
- `unknown` - The environment is in an unknown state.

## Entering a shell

You can spawn a shell within an environment by running the following command.

```
dda env dev shell
```

This opens a shell within the first defined [repository](#repository-selection).

## Preparing for development

Run the following command in the environment's shell to install some remaining dependencies.

```
dda inv install-tools
```

If you're using a [remote clone](#remote-clone), some builds may fail due to the shallow cloning. To acquire all Git history, run the following command.

```
git fetch --unshallow
```

## Testing

Verify your setup by running some [unit tests](../../how-to/test/unit.md) with the following command.

```
dda inv test --targets=pkg/aggregator
```

## Editing code

Exit the environment's shell or open a new session and run the following command locally.

```
dda env dev code
```

This opens one of the [supported editors](../../reference/images/dev.md#editors) for the repository selected with the `-r`/`--repo` option, defaulting to the first defined [repository](#repository-selection). The editor may take a few moments to start up the first time it opens in an environment.

The editor may be selected with the `-e`/`--editor` option or by [configuring][dda-config] `dda` to use a specific [editor](https://datadoghq.dev/datadog-agent-dev/reference/api/config/#dda.config.model.env.DevEnvConfig.editor) for developer environments by default with the following command.

```
dda config set env.dev.editor cursor
```

To test functionality, create a file `test.txt` at the root of the repository and save it.

## Running commands

You can run commands within an environment without [entering a shell](#entering-a-shell) by passing arbitrary arguments to the `dda env dev run` command locally. Confirm the file `test.txt` was created by running the following command.

```
dda env dev run -- ls -l test.txt
```

The repository in which to run the command is determined by the `-r`/`--repo` option, defaulting to the first defined [repository](#repository-selection).

## Building

Let's [build](../../how-to/build/standalone.md) the default Agent by running the following command locally.

```
dda env dev run -- dda inv agent.build --build-exclude=systemd
```

Confirm that the binary was successfully built by running the following command.

```
dda env dev run -- ls -l bin/agent/agent
```

If you're using a [local checkout](#local-checkout) you should also see that binary on your local machine. This strategy extends to other build artifacts so you could, for example, build the Debian package and install it on an arbitrary machine running Ubuntu.

/// note | Caveat
Environments using a [remote clone](#remote-clone) have no easier way to share build artifacts with your local machine than to use the `docker cp` command. This is a temporary limitation.
///

## Stopping environments

You can stop an environment by running the following command.

```
dda env dev stop
```

This stops the environment but does not remove it. Environments can be [started](#starting-environments) again at any time from a [stopped state](#environment-status). When this happens, the start command only accepts options for [environment selection](#environment-selection).

## Removing environments

Environments can be removed by running the following command.

```
dda env dev remove
```

This removes the environment and all associated data.

/// tip
The stop command accepts a `-r`/`--remove` option to remove the environment after stopping it, useful for ephemeral environments.

```
dda env dev stop -r
```
///
