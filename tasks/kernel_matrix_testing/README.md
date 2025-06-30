# Kernel Matrix Testing (KMT)

*   [Overview](#overview)
*   [Dependencies](#dependencies)
*   [Quick Start](#quick-start)
    *   [Quick Start for Local VMs](#quick-start-for-local-vms)
    *   [Quick Start for Remote VMs](#quick-start-for-remote-vms)
    *   [Quick Start for Local and Remote VMs](#quick-start-for-local-and-remote-vms)
*   [Core Concepts](#core-concepts)
    *   [Stacks](#stacks)
    *   [VMsets File](#vmsets-file)
    *   [Local vs. Remote VMs](#local-vs-remote-vms)
*   [Updating and Adding Resources](#updating-and-adding-resources)
*   [Command Reference](#command-reference)
    *   [kmt.init](#kmtinit)
    *   [kmt.update-resources](#kmtupdate-resources)
    *   [kmt.ls](#kmtls)
    *   [kmt.gen-config](#kmtgen-config)
    *   [kmt.launch-stack](#kmtlaunch-stack)
    *   [kmt.status](#kmtstatus)
    *   [kmt.ssh-config](#kmtssh-config)
    *   [kmt.tmux](#kmttmux)
    *   [kmt.build](#kmtbuild)
    *   [kmt.test](#kmttest)
    *   [kmt.pause](#kmtpause)
    *   [kmt.resume](#kmtresume)
    *   [kmt.destroy-stack](#kmtdestroy-stack)
    *   [kmt.config-ssh-key](#kmtconfig-ssh-key)
    *   [kmt.explain-ci-failure](#kmtexplain-ci-failure)
*   [Advanced Usage](#advanced-usage)
    *   [Connecting to VMs](#connecting-to-vms)
    *   [CI Integration](#ci-integration)
    *   [Using with External ("Alien") VMs](#using-with-external-alien-vms)
*   [Troubleshooting](#troubleshooting)
    *   [macOS Network Issues](#macos-network-issues)

## Overview

The Kernel Matrix Testing (KMT) system is a new approach for testing `system-probe`. It uses `libvirt` and `qemu` to launch pre-provisioned VMs over a range of distributions and kernel versions. These VMs are used for running the `system-probe` test suite.

Developers can check out [this](https://datadoghq.atlassian.net/wiki/spaces/EBPFTEAM/pages/3278832713/Developer+Documentation) confluence page for more details about the system.

This document covers the `invoke` tasks provided to manage the lifecycle of the VMs.

> All commands are assumed to be executed from the root directory of the `datadog-agent` repository.

## Dependencies

All dependencies are installed by the `kmt.init` command. See [kmt.init](#kmtinit) for details.

For macOS users, see the [macOS Network Issues](#macos-network-issues) section in troubleshooting.

## Quick Start

### Quick Start for Local VMs

This guide shows a minimal workflow to get a single local VM running.

**1. Initialize KMT** (only needs to be done once)

This will download required resources and install system dependencies.

> **Note:** `kmt.init` resets your KMT environment. If you have an existing setup, use `kmt.update-resources` to download additional images.

```bash
# Initialize with a specific VM image
dda inv -e kmt.init --images=ubuntu_22.04

# Or download all available VM images for your architecture (this can be slow)
dda inv -e kmt.init --all-images
```

**2. Create and Configure a Stack**

A stack is a collection of VMs. This command will create a new stack and configure it to run one local Ubuntu 22.04 VM.

```bash
# If --stack is not provided, a name is generated from your git branch
dda inv -e kmt.gen-config --vms=ubuntu_22.04-local-distro
```

**3. Launch the Stack**

This will start the VM(s) in your stack.

```bash
dda inv -e kmt.launch-stack
```
> Since this is a local VM, you may be prompted for your password to run some commands with `sudo`.

**4. Check Status**

You can view the running VMs and their IP addresses in the current stack.

```bash
dda inv -e kmt.status
```

**5. Interact with the VM**

The primary way to interact with the VMs is by running tests using the `kmt.test` command.

```bash
# Run all tests on the VM (this will run ALL tests, which can take a while)
dda inv -e kmt.test --vms=ubuntu_22.04-local-distro

# Run a specific test (recommended for development)
dda inv -e kmt.test --vms=ubuntu_22.04-local-distro --packages ./pkg/network/usm/tests --run TestFullMonitorWithTracer
```

Optionally, for debugging or manual inspection, you can connect to the VM directly using SSH. The easiest way is to generate an SSH configuration file. The stack name will be automatically inferred from your git branch.

```bash
# Generate the SSH config
dda inv -e kmt.ssh-config > ~/.ssh/kmt_ssh_config

# Add `Include ~/.ssh/kmt_ssh_config` to your main ~/.ssh/config file.
# Then, you can connect with:
ssh kmt-<your-branch-name>-local-ubuntu_22.04
```

**6. Clean Up**

When you are finished, destroy the stack to tear down all associated resources.

```bash
dda inv -e kmt.destroy-stack
```

### Quick Start for Remote VMs

This guide shows a minimal workflow to get a single remote VM running in AWS.

**1. Initialize KMT & Configure SSH Key** (only needs to be done once)

To create EC2 instances for remote VMs, KMT needs an SSH key to provision them. The `kmt.init` command will guide you through an interactive wizard to set up a default SSH key, so you don't have to provide it for every command. For a detailed explanation of the supported methods, see the [`kmt.config-ssh-key`](#kmtconfig-ssh-key) command documentation.

> **Note:** `kmt.init` resets your KMT environment. If you have an existing setup, consider using `kmt.config-ssh-key` to configure an SSH key without re-initializing.

```bash
# Using --remote-setup-only is sufficient if you don't plan to use local VMs
dda inv -e kmt.init --remote-setup-only
```
> **Note:** For day-to-day development, using local VMs is highly recommended as remote VMs launch metal instances which can be costly. Remote VMs are best suited for running tests against a large number of VMs concurrently.

> If you have already run `kmt.init`, you can configure the SSH key separately at any time by running `dda inv -e kmt.config-ssh-key`.

**2. Create and Configure a Stack**

This command will create a new stack and configure it to run one remote Ubuntu 22.04 VM on an x86_64 EC2 instance.

```bash
dda inv -e kmt.gen-config --vms=x86-jammy-distro
```

You can also specify multiple VMs with different architectures. KMT will launch them on separate EC2 instances.

```bash
# This configures a stack with multiple x86_64 and arm64 VMs
dda inv -e kmt.gen-config --vms=x86-jammy-distro,x86-focal-distro,arm64-amazon4.14-distro,arm64-amazon5.10-distro
```

**3. Launch the Stack**

This will launch the EC2 instance and boot the VM. Your pre-configured SSH key will be used automatically.

```bash
dda inv -e kmt.launch-stack
```

**4. Check Status**

You can view the running VMs and their IP addresses in the current stack. It may take a few moments for the VM to be assigned an IP.

```bash
dda inv -e kmt.status
```

**5. Interact with the VM**

The primary way to interact with the VMs is by running tests using the `kmt.test` command.

```bash
# Run all tests on the VM (this will run ALL tests, which can take a while)
dda inv -e kmt.test --vms=x86-jammy-distro

# Run a specific test (recommended for development)
dda inv -e kmt.test --vms=x86-jammy-distro --packages ./pkg/network/usm/tests --run TestFullMonitorWithTracer
```

Optionally, for debugging or manual inspection, you can connect to the VM directly using SSH. The easiest way is to generate an SSH configuration file. The stack name will be automatically inferred from your git branch.

```bash
# Generate the SSH config
dda inv -e kmt.ssh-config > ~/.ssh/kmt_ssh_config

# Add `Include ~/.ssh/kmt_ssh_config` to your main ~/.ssh/config file if you haven't.
# Then, you can connect with:
ssh kmt-<your-branch-name>-distro-ubuntu_22.04
```

**6. Clean Up**

When you are finished, destroy the stack to terminate the EC2 instance.

```bash
dda inv -e kmt.destroy-stack
```

### Quick Start for Local and Remote VMs

This guide shows how to run a mixed stack with both local and remote VMs.

**1. Initialize KMT for a Full Setup** (only needs to be done once)

To run both local and remote VMs, you need to download VM images and configure an SSH key. The `kmt.init` command with an `--images` flag will handle both.

> **Note:** `kmt.init` resets your KMT environment. If you have an existing setup, use `kmt.update-resources` to download additional images and `kmt.config-ssh-key` to configure SSH keys.

```bash
# This will download the Ubuntu 22.04 image and start the SSH key setup wizard
dda inv -e kmt.init --images=ubuntu_22.04
```

**2. Create and Configure a Mixed Stack**

Provide a list of both local and remote VMs to the `gen-config` command.

```bash
# This configures one local Ubuntu 22.04 VM and one remote x86_64 Ubuntu 22.04 VM
dda inv -e kmt.gen-config --vms=ubuntu_22.04-local-distro,x86-jammy-distro
```

**3. Launch the Stack**

The same `launch-stack` command will start the local VM and launch the remote EC2 instance.

```bash
dda inv -e kmt.launch-stack
```
> You may be prompted for your `sudo` password for the local VM setup. The remote VM will use your pre-configured SSH key.

**4. Connect and Clean Up**

The `status`, `ssh-config`, and `destroy-stack` commands work just like in the other scenarios, managing all VMs in the stack together.

```bash
# Check status of all VMs
dda inv -e kmt.status

# When finished, destroy all resources
dda inv -e kmt.destroy-stack
```

## Core Concepts

### Stacks

The system works on the concept of `stacks`. A `stack` is a collection of VMs, both local and remote, that are managed together. A `stack` is given a unique name by the user. If no stack name is provided, a name is generated automatically from the current git branch name. This allows developers to couple `stacks` with their git workflow.

A `stack` can be:

-   Created
-   Configured
-   Launched
-   Paused
-   Resumed
-   Destroyed

### VMsets File

Configuring a stack involves generating a configuration file, referred to as the `vmsets` file. This JSON file specifies the sets of VMs to launch. A "set" of VMs is a collection of VMs sharing characteristics like architecture or purpose (e.g., "all x86_64 distribution VMs on a remote machine").

This file is managed by the `kmt.gen-config` command. You typically don't need to edit it manually.

A sample VMSet file can be found [here](https://github.com/DataDog/test-infra-definitions/blob/f85e7eb2f003b6f9693c851549fbb7f3969b8ade/scenarios/aws/microVMs/sample-vm-config.json).

### Local vs. Remote VMs

KMT can manage two types of VMs:

-   **Local VMs**: These run directly on your machine using `libvirt` and `qemu`. They will have the same architecture as your host machine.
-   **Remote VMs**: These are EC2 instances launched in AWS. KMT can manage both x86_64 and arm64 remote VMs. Launching remote VMs requires configuring an SSH key.

## Updating and Adding Resources

A common task is to download new VM images or update existing ones. It is important to choose the right command to avoid unintended consequences.

*   **`kmt.init`**: This command is for the initial setup of the KMT environment. As noted previously, running it again will reset the entire KMT state, including configurations and previously downloaded resources. You should only use this for a first-time install or if you want to start from a completely clean slate.

*   **`kmt.update-resources`**: This is the correct command to use when you want to download additional VM images or update existing ones to their latest versions. Unlike `kmt.init`, it does not reset your entire KMT configuration.

The `update-resources` command is efficient and avoids re-downloading data unnecessarily. It works by:
1.  Checking for the presence of local VM images.
2.  Comparing the checksums of local images against the remote ones.
3.  Downloading only the images that are missing or have been updated.

> **Warning:** To ensure a consistent state, `kmt.update-resources` will first destroy all currently running stacks before starting the download process. Make sure you have saved any work and are ready to tear down your active VMs.

Example:
```bash
# Download a new debian image without resetting everything
dda inv -e kmt.update-resources --images=debian_11
```

## Command Reference

This section provides a detailed reference for all `kmt` tasks.

### `kmt.init`

Initializes the KMT environment. It downloads VM images and tools, and installs system dependencies.

> Running `kmt.init` will overwrite previous configuration and downloaded resources. Use `kmt.update-resources` instead to fetch additional images.

This command resets the entire KMT state, including the SSH configuration and any prior downloaded images.

```bash
# Initialize with specific VM images
dda inv -e kmt.init --images=ubuntu_22.04,debian_10

# Initialize and download all available VM images for your architecture
dda inv -e kmt.init --all-images

# If you only intend to manage remote VMs
dda inv -e kmt.init --remote-setup-only
```

The `--images` parameter is required unless `--remote-setup-only` or `--all-images` is specified, or you explicitly confirm the interactive prompt. Use `kmt.ls` to see available images.

This command will also guide you through the [default SSH key configuration](#kmtconfig-ssh-key).

### `kmt.update-resources`

Downloads new or updates existing VM images. For a detailed explanation of when and how to use this command, see the [Updating and Adding Resources](#updating-and-adding-resources) section.

> **Warning:** This command will first destroy all running stacks before downloading the images.

```bash
# Update all available images
dda inv -e kmt.update-resources

# Update only specific images
dda inv -e kmt.update-resources --images=ubuntu_22.04,debian_11
```

### `kmt.ls`

Lists available VM images and indicates which ones are already downloaded locally.

```bash
dda inv -e kmt.ls
```

### `kmt.gen-config`

Generates or updates the `vmsets` configuration file for a stack, specifying which VMs to launch.

```bash
dda inv -e kmt.gen-config --vms=<list> [--stack=<name>]
```

**Common Flags:**

-   `--vms=<list>`: A comma-separated list of VMs to add to the stack. See [Specifying VMs](#specifying-vms) below for the format.
-   `--init-stack`: Automatically creates the stack if it doesn't exist. Stack name is generated from the current git branch name, if not explicitly provided. This is true by default.
-   `--stack=<name>`: The target stack. Defaults to a name based on the current git branch.
-   `--new`: Creates a fresh configuration file, removing any existing VMs from the stack's configuration.
-   `--from-ci-pipeline=<id>`: Configures the stack to replicate failed jobs from a CI pipeline. See [CI Integration](#ci-integration).

**Specifying VMs (`--vms`)**

The `--vms` list is a comma-separated list of VM entries. Each entry is parsed in a fuzzy manner and must contain three elements separated by dashes (`-`):

1.  **Recipe**: `distro` for distribution images or `custom` for custom kernels.
2.  **Architecture**: `x86_64` (or `amd64`), `arm64` (or `aarch64`), or `local` for your machine's architecture.
3.  **Version**: The distribution name/version (e.g., `ubuntu22`, `jammy`, `debian10`) or a custom kernel version (e.g., `5.4`).

The order of elements does not matter.

**Examples:**

-   `jammy-local-distro`, `distro-local-jammy`, `local-ubuntu22-distro` all resolve to a **local Ubuntu 22.04 distribution VM**.
-   `amazon4.14-x86-distro`, `distro-x86_64-amazon4.14` all resolve to a **remote x86_64 Amazon Linux 2 (kernel 4.14) distribution VM**.
-   `custom-arm-5.4`, `5.4-arm64-custom` all resolve to a **remote arm64 custom kernel 5.4 VM**.

### `kmt.launch-stack`

Launches all configured VMs in a stack. If VMs have been added to the configuration since the last launch, it will only start the new ones.

```bash
dda inv -e kmt.launch-stack [--stack=<name>] [--ssh-key=<key>]
```

-   If launching **local VMs**, you may be prompted for your `sudo` password. This is required because the program needs to run some commands as root. We do not run the entire scenario with `sudo` to avoid creating broken permissions.
-   If launching **remote VMs**, an SSH key is required. If you configured a default key with `kmt.config-ssh-key`, you don't need to provide it again.

The `--ssh-key` argument allows you to specify a key for this launch, overriding the default. The value can be:

-   A path to a private key file (e.g., `~/.ssh/my_key`).
-   The filename of a private key in `~/.ssh` (e.g., `id_ed25519`).
-   A key name/comment. KMT will search for a matching public key in `~/.ssh/*.pub` and your SSH agent.

### `kmt.status`

Prints information about a stack, including running VMs and their IP addresses.

```bash
dda inv -e kmt.status [--stack=<name>]
```

### `kmt.ssh-config`

Generates an SSH configuration file for connecting to the VMs in a stack.

```bash
dda inv -e kmt.ssh-config [--stack=<name>]
```

This is the recommended way to connect to VMs. See [Connecting to VMs](#connecting-to-vms) for usage.

### `kmt.tmux`

Connects to all VMs in a stack within a new `tmux` session.

It creates a new session for your stack (deleting any existing one), opens a new window for each instance (e.g., local, remote x86), and a new pane for each VM.

A useful tmux command in this context is `:set synchronize-panes on` to send commands to all VMs simultaneously.

### `kmt.build`

Builds `system-probe` locally and shares it with the specified VMs in a stack.

```bash
dda inv -e kmt.build --vms=<vms-list> [--stack=<name>]
```

This command shares a [default configuration](./default-system-probe.yaml) for `system-probe` with all the VMs.

### `kmt.test`

Runs the `system-probe` test suite on the specified VMs.

```bash
# Run all tests on a specific VM
dda inv -e kmt.test --vms=jammy-local-distro [--stack=<name>]

# Run specific tests in a specific package
dda inv -e kmt.test --vms=jammy-local-distro --packages=./pkg/network/tracer --run="TestTracerSuite/prebuilt/TestDNSStats/valid_domain"
```

Refer to the task help for more parameters: `dda inv --help=kmt.test`.

### `kmt.pause`

Pauses a stack, stopping all local VMs and freeing their resources. The VM disk state is preserved.

This has no effect on remote VMs.

```bash
dda inv -e kmt.pause [--stack=<name>]
```

### `kmt.resume`

Resumes a previously paused stack, restarting the local VMs.

```bash
dda inv -e kmt.resume [--stack=<name>]
```

### `kmt.destroy-stack`

Destroys a stack, tearing down all associated resources. This deletes local VMs and terminates remote EC2 instances. **This action is irreversible.**

This command will attempt to manually tear down all resources, primarily cleaning the local `libvirt` environment and destroying remote EC2 instances.

```bash
dda inv -e kmt.destroy-stack [--stack=<name>]
```

### `kmt.config-ssh-key`

Launches an interactive wizard to configure the default SSH key for launching remote VMs.

```bash
dda inv -e kmt.config-ssh-key
```

This configuration is stored globally for KMT, so you don't have to provide the `--ssh-key` argument to `kmt.launch-stack` every time. The wizard supports three methods for locating your key:

-   **Keys in `~/.ssh`**: Automatically finds key files in your `.ssh` directory for you to choose from.
-   **Keys in SSH Agent**: Finds keys loaded in an SSH agent (e.g., the 1Password SSH agent). Note that with this method, KMT does not know the path to the private key file, which means the generated SSH config cannot specify which key to use. This can be problematic if you have many keys in the agent, as SSH may try all of them and be rejected before it finds the correct one. In that case, you may need to configure it manually. [See more details in the 1Password documentation](https://developer.1password.com/docs/ssh/agent/advanced/).
-   **Manual Path**: Provide the full path to your private key file. Note that **this method performs no validation on the provided path**.

In all cases, you can also specify the key name registered in AWS in case it differs from your local key's name.

### `kmt.explain-ci-failure`

Provides a summary of test failures in a CI pipeline.

```bash
dda inv -e kmt.explain-ci-failure <pipeline-id>
```
This command analyzes a given pipeline and reports on job failures (distinguishing between infra and test failures), test failures across different distributions, and potential boot issues for infra failures.

## Advanced Usage

### Connecting to VMs

#### Using automatic SSH config (Recommended)

You can generate an SSH config file to easily connect to the VMs:

```bash
dda inv -e kmt.ssh-config --stack=<stack-name> > ~/.ssh/kmt_ssh_config
```

This will generate a file `kmt_ssh_config` in your `~/.ssh` directory. To enable it, add the line `Include ~/.ssh/kmt_ssh_config` to your main `~/.ssh/config` file.

You can then connect to the VMs using a standard format:

```bash
ssh kmt-<stack-name>-<instance-type>-<vm-name>

# Example for a remote VM:
ssh kmt-demo-stack-distro-ubuntu_22.04

# Example for a local VM:
ssh kmt-demo-stack-local-ubuntu_22.04
```

This approach automatically handles proxy jumps, user accounts, SSH keys, and disables strict host key checking, which is convenient for ephemeral VMs.

#### Manually using IPs

You can get VM IP addresses with `kmt.status` and connect manually.

```bash
# Connect to a remote VM (first SSH to the EC2 instance)
ssh -i /home/kernel-version-testing/ddvm_rsa -o StrictHostKeyChecking=no root@<ip>
```

### CI Integration

#### Replicating Failed CI Jobs

You can easily configure a stack to replicate the jobs that failed in a CI pipeline using the `--from-ci-pipeline` flag.

```bash
dda inv -e kmt.gen-config --from-ci-pipeline=<pipeline-id> --use-local-if-possible --stack=demo-stack
```

The `--use-local-if-possible` flag will try to use local VMs for the failed jobs if the architecture matches, falling back to remote VMs otherwise.

#### Analyzing CI Failures

Use the [`kmt.explain-ci-failure`](#kmtexplain-ci-failure) command to get a detailed summary of failures in a given pipeline.

### Using with External ("Alien") VMs

You can use KMT tasks like `kmt.build` and `kmt.test` with VMs that are not managed by KMT (e.g., VMs from VMware, Parallels, or other cloud providers). To do this, you create a JSON "alien profile" file.

The profile is a JSON list of objects, where each object represents a VM with the following required fields: `ssh_key_path`, `ip`, `arch`, `name`, and `ssh_user`.

**Example `alien.profile`:**

```json
[
    {
        "ssh_key_path": "/home/user/.ssh/some-key.id_rsa",
        "ip": "xxx.yyy.aaa.bbb",
        "arch": "x86",
        "name": "ubuntu-gcp",
        "ssh_user": "ubuntu"
    }
]
```

Then, use the `--alien-vms` flag to provide the path to this profile file.

```bash
dda inv -e kmt.build --alien-vms=/tmp/alien.profile
dda inv -e kmt.test --packages=./pkg/ebpf --run=TestLockRanges/Hashmap --alien-vms=./alien.profile
```

## Troubleshooting

### macOS Network Issues

For macOS users, networking for local VMs may not work correctly unless **Internet Sharing** is enabled.

You can enable it in `System Settings > General > Sharing`. The interface you share from does not matter, as long as the feature is active for your main internet connection. We would appreciate it if you reported any occurrences of this to the eBPF platform team, as it's not yet clear why this is sometimes required.
