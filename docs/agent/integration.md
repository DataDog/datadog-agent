# Integrations management

The agent comes with a set of bundled official Datadog integrations to allow users to quickly start monitoring their applications.
These integrations are also available as single python packages that can be individually upgraded.
The `datadog-agent integration` command allows users to easily and securely manage the official Datadog integrations that are available for the agent.
It has several subcommands:
 - [install](#install-an-integration)
 - [remove](#remove-an-integration)
 - [show](#show-information-about-an-integration)
 - [freeze](#list-all-python-packages)

The usage and documentation of these commands can be printed with `datadog-agent integration --help`.
On linux, the command needs to be executed as the `dd-agent` user, and as `administrator` on windows.

## Recommended usage and workflow

1. Check out the version of the integration you have installed in your agent with the `show` command
1. Look at the changelog of the integration on the [integrations-core][1] repository to identify the version you want and make sure the changes correspond to what you want
1. Install the integration with the `install` command
1. Restart your agent

**Note** When using a configuration management tool, it is recommended to pin the integration to the desired version, and when the agent is upgraded, to remove the pin of the individual integration.
Otherwise, upgrading the agent without removing the pin on the individual integration will likely cause the configuration management tool to fail since the version of the integration will likely not be compatible with the new version of the agent.

## Install an integration

With the `datadog-agent integration install` command, you can install a specific version of an official Datadog integration (available on the [integrations-core repository][1]), provided that it is compatible with the version of the agent. The command does this verification and exits with a failure in case of incompatibilities.

An integration is compatible and installable if:
 1. Its version is newer than the one [shipped with the agent][2].
 1. It is compatible with the version of the [datadog_checks_base][3].
 1. It is not `datadog_checks_base`. The base check can only be upgraded by upgrading the agent.

The syntax for this command is `datadog-agent integration install <integration_package_name>==<version>` where `<integration_package_name>` is the name of the integration prefixed with `datadog-`.

For instance, to install version 3.6.0 of the vSphere integration, run:

Linux:
```
sudo -u dd-agent -- datadog-agent integration install datadog-vsphere==3.6.0
```
Windows:
```
"C:\Program Files\Datadog\Datadog Agent\embedded\agent.exe" integration install datadog-vsphere==3.6.0
```

The command installs the python package of the integration and then copy configuration files (`conf.yaml.example`, `conf.yaml.default`, `auto_conf.yaml`) to the `conf.d` directory, overwriting the existing ones like what is done during a full agent upgrade.
In case of failure during the copy of those files, the command exit with a failure, but the version of the integration you specified still gets installed.

After upgrading, restart your agent to begin using the newly installed integration.

This command is designed specifically to allow users to upgrade an individual integration to get a new feature or bugfix as soon as it is available, without needing to wait for the next release of the agent.
That said, it is still recommended to upgrade the agent when it is possible, as it always ships the latest version of every integration at the time of the release.

Upon agent upgrade, every integration that you individually upgraded using the command gets overwritten by the integration shipped within the agent.

### Configuration management tools

Configuration management tools can leverage this command to deploy the version of an integration across your entire infrastructure.

## Remove an integration

To remove an integration, use the `datadog-agent integration remove` command.
The syntax for this command is `datadog-agent integration remove <integration_package_name>` where `<integration_package_name>` is the name of the integration prefixed with `datadog-`.

For instance, to remove the vSphere integration, run:

Linux:
```
sudo -u dd-agent -- datadog-agent integration remove datadog-vsphere
```
Windows:
```
"C:\Program Files\Datadog\Datadog Agent\embedded\agent.exe" integration remove datadog-vsphere
```

Removing an integration does not remove the corresponding configuration folder in the `conf.d` directory.

## Show information about an integration

To get information, such as the version, about an installed integration, use the `datadog-agent integration show` command.
The syntax for this command is `datadog-agent integration show <integration_package_name>` where `<integration_package_name>` is the name of the integration prefixed with `datadog-`.

For instance, to show information on the vSphere integration, run:

Linux:
```
sudo -u dd-agent -- datadog-agent integration show datadog-vsphere
```
Windows:
```
"C:\Program Files\Datadog\Datadog Agent\embedded\agent.exe" integration show datadog-vsphere
```

## List all python packages

To list all the python packages installed in the agent's python environment, use the `datadog-agent integration freeze` command.
This will list all the Datadog integrations (packages starting with `datadog-`) as well as all the python dependencies required to run the integrations.

Linux:
```
sudo -u dd-agent -- datadog-agent integration freeze
```
Windows:
```
"C:\Program Files\Datadog\Datadog Agent\embedded\agent.exe" integration freeze
```

[1]: https://github.com/DataDog/integrations-core
[2]: https://github.com/DataDog/integrations-core/blob/master/AGENT_INTEGRATIONS.md
[3]: https://github.com/DataDog/integrations-core/tree/master/datadog_checks_base
