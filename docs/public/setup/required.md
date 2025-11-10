# Set up development requirements

-----

## Tooling

The `dda` [CLI](https://datadoghq.dev/datadog-agent-dev/) is required in all aspects of development and must be available on `PATH`.

<<<DDA_DOCS_INSTALL>>>

## Docker

[Docker](https://docs.docker.com/get-started/docker-overview/) is required for both running the developer environment and building images containing the Agent.

/// tab | macOS
1. Install [Docker Desktop for Mac](https://docs.docker.com/desktop/setup/install/mac-install/).
1. Right-click the Docker taskbar item and update **Preferences > File Sharing** with any locations you need to open.
///

/// tab | Windows
1. Install [Docker Desktop for Windows](https://docs.docker.com/desktop/setup/install/windows-install/).
1. Right-click the Docker taskbar item and update **Settings > Shared Drives** with any locations you need to open e.g. `C:\`.
///

/// tab | Linux
Install Docker Desktop for your distribution:

//// tab | Ubuntu
[Docker Desktop for Ubuntu](https://docs.docker.com/desktop/setup/install/linux/ubuntu/)
////

//// tab | Debian
[Docker Desktop for Debian](https://docs.docker.com/desktop/setup/install/linux/debian/)
////

//// tab | Fedora
[Docker Desktop for Fedora](https://docs.docker.com/desktop/setup/install/linux/fedora/)
////

//// tab | Arch
[Docker Desktop for Arch](https://docs.docker.com/desktop/setup/install/linux/archlinux/)
////

//// tab | RHEL
[Docker Desktop for RHEL](https://docs.docker.com/desktop/setup/install/linux/rhel/)
////
///

## SSH

Accessing and contributing to the Datadog Agent repositories requires using [SSH](https://docs.github.com/en/authentication/connecting-to-github-with-ssh/about-ssh). Your local SSH agent must be [configured](https://docs.github.com/en/authentication/connecting-to-github-with-ssh/generating-a-new-ssh-key-and-adding-it-to-the-ssh-agent) with a key that is [added](https://docs.github.com/en/authentication/connecting-to-github-with-ssh/adding-a-new-ssh-key-to-your-github-account) to your GitHub account.

If these requirements are met, you can skip this section.

### Key generation

Run the following command to generate a new SSH key, replacing `<EMAIL_ADDRESS>` with one of the email addresses associated with your GitHub account. This email will be used for every Git commit you make to the Datadog Agent repositories.

```
ssh-keygen -t ed25519 -C "<EMAIL_ADDRESS>"
```

It should then ask you for the path in which to save the key. It's recommended to name the key `dda` and save it in the same directory as the default location.

```
Enter file in which to save the key (/root/.ssh/id_ed25519): /root/.ssh/dda
```

Finally, you will be asked to enter an optional [passphrase](https://docs.github.com/en/authentication/connecting-to-github-with-ssh/working-with-ssh-key-passphrases).

### Key awareness

Add the key from the previous step to your local SSH agent.

```
ssh-add /root/.ssh/dda
```

Running the following command should now display the key.

```
ssh-add -L
```

Follow GitHub's [guide](https://docs.github.com/en/authentication/connecting-to-github-with-ssh/adding-a-new-ssh-key-to-your-github-account) for adding the key to your GitHub account and [test](https://docs.github.com/en/authentication/connecting-to-github-with-ssh/testing-your-ssh-connection) that it works. The path to the public key is the path to the key from the previous step with a `.pub` file extension e.g. `/root/.ssh/dda.pub`.

## Next steps

Follow the developer environment [tutorial](../tutorials/dev/env.md) to get started.
