# Developer environment reference

-----

The developer environments are [container images](https://github.com/DataDog/datadog-agent-buildimages/tree/main/dev-envs) that use the [builders](builders.md) as a base image. These images are only intended for use by the [`dda env dev`](https://datadoghq.dev/datadog-agent-dev/reference/cli/commands/#dda-env-dev) command group.

There is currently only one image available:

- [`datadog/agent-dev-env-linux`](https://hub.docker.com/r/datadog/agent-dev-env-linux), based on the [Linux builder](builders.md#linux)

## Editors

Images come with the following editors ready for remote development via SSH.

- [Visual Studio Code](https://code.visualstudio.com)
- [Cursor](https://www.cursor.com)

/// details | Pre-installed extensions
//// tab | VS Code/Cursor
<<<VSCODE_EXTENSIONS>>>
////
///

## Tools

The following is a non-exhaustive list of available tools.

- [`ambr`](https://github.com/dalance/amber) - find and replace like `sed` but with interactivity
- [`bat`](https://github.com/sharkdp/bat) - show file contents like `cat`
- [`bazel`](https://bazel.build) - [Bazelisk](https://github.com/bazelbuild/bazelisk), ensuring the correct version of the Bazel build system is used
- [`btm`](https://github.com/ClementTsang/bottom) - system monitor like `top`
- [`dda`](https://github.com/DataDog/datadog-agent-dev) - Datadog Agent development CLI
- [`docker`](https://docs.docker.com/engine/) - Docker engine, including the `buildx` and `compose` plugins
- [`eza`](https://github.com/eza-community/eza) - list files like `ls`
- [`fd`](https://github.com/sharkdp/fd) - find files like `find`
- [`fzf`](https://github.com/junegunn/fzf) - fuzzy finder
- [`gfold`](https://github.com/nickgerace/gfold) - Git status viewer for multiple repositories
- [`gitui`](https://github.com/extrawurst/gitui) - Git terminal UI
- [`hyperfine`](https://github.com/sharkdp/hyperfine) - benchmarking tool
- [`kubectl`](https://kubernetes.io/docs/reference/kubectl/) - Kubernetes control plane CLI
- [`jq`](https://github.com/jqlang/jq) - JSON processor
- [`pdu`](https://github.com/KSXGitHub/parallel-disk-usage) - show disk usage like `du`
- [`procs`](https://github.com/dalance/procs) - process viewer like `ps`
- [`rg`](https://github.com/BurntSushi/ripgrep) - search file contents like `grep`
- [`yazi`](https://github.com/sxyazi/yazi) - file manager UI

## Git

Git is configured to use [SSH](../../setup/required.md#ssh) for authentication and, upon start, will use the `user.name` and `user.email` global settings from your local machine.

Each of the following subcommands are available via `git <subcommand>`.

- `dd-clone` - Performs a shallow clone [^1] of a Datadog repository to the proper managed location. The first argument is the repository name and a second optional argument is the branch name. Example invocations:
    - `git dd-clone datadog-agent`
    - `git dd-clone datadog-agent user/feature`
- `dd-switch` - Emulates the behavior of `git switch` but smart enough to handle shallow clones. The branch name is the only argument.

[Delta](https://github.com/dandavison/delta) is the default pager which provides, as an example, syntax highlighting for `git diff` and `git log` commands.

## Repositories

Images assume repositories will be cloned to `~/repos`. The [`dd-clone`](#git) Git extension will clone repositories to this location and `gfold` is pre-configured to look for repositories in this location.

## Shells

Images come with shells based on their platform e.g. [Zsh](https://www.zsh.org) for Linux and [PowerShell](https://github.com/PowerShell/PowerShell) for Windows. Every image comes with [Nushell](https://github.com/nushell/nushell) as well.

All shells are pre-configured with [Starship](https://github.com/starship/starship) prompt and any local configuration will be copied to the container upon start.

## Fonts

Images come with the following fonts.

- [FiraCode](https://github.com/ryanoasis/nerd-fonts)
- [CascadiaCode](https://github.com/microsoft/cascadia-code)

All fonts have [Nerd Font](https://www.nerdfonts.com) glyphs, and [Noto Emoji](https://github.com/googlefonts/noto-emoji) is installed for emoji support.

## Ports

Images expose the following ports.

- `22` for SSH access
- `9000` for the `dda` MCP server

[^1]: A shallow clone by default matches our use case of ephemeral developer environments. If persistence is desired then developers can easily convert the shallow clone to a full clone by running `git fetch --unshallow`. More information:

      - [Git clone: a data-driven study on cloning behaviors](https://github.blog/open-source/git/git-clone-a-data-driven-study-on-cloning-behaviors/)
      - [Get up to speed with partial clone and shallow clone](https://github.blog/open-source/git/get-up-to-speed-with-partial-clone-and-shallow-clone/)
