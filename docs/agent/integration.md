# Integrations management

## Overview

The Agent comes with a set of bundled official Datadog integrations to allow users to quickly start monitoring their applications. These integrations are also available as single Python packages that can be individually upgraded.

For Agent v6.8+, the `datadog-agent integration` command allows users to manage the official Datadog integrations that are available for the Agent. It has the following subcommands:

 * [install](#install)
 * [remove](#remove)
 * [show](#show)
 * [freeze](#freeze)

The usage and documentation of these commands can be printed with `datadog-agent integration --help`.  
For Linux, execute the command as the `dd-agent` user. For Windows, execute the command as an `administrator`.

## Integration commands

### Summary workflow

1. Check the version of the integration installed in your Agent with the `show` command.
2. Review the changelog of the specific integration on the [integrations-core][1] repository to identify the version you want.
3. Install the integration with the `install` command.
4. Restart your Agent.

**Note**: When using a configuration management tool, it is recommended to pin the integration to the desired version. When the Agent is upgraded, remove the integration pin. Upgrading the Agent without removing the integration pin can cause the configuration management tool to fail if the version of the integration is not compatible with the new version of the Agent.

### Install

Use the `datadog-agent integration install` command to install a specific version of an official Datadog integration (available on the [integrations-core repository][1]), provided that it is compatible with the version of the Agent. The command does this verification and exits with a failure in case of incompatibilities.

An integration is compatible and installable if:
 1. The version is newer than the one [shipped with the Agent][2].
 1. It is compatible with the version of the [datadog_checks_base][3].
 1. It is not the `datadog_checks_base`. The base check can only be upgraded by upgrading the Agent.

The syntax for this command is `datadog-agent integration install <integration_package_name>==<version>` where `<integration_package_name>` is the name of the integration prefixed with `datadog-`.

For example, to install version 3.6.0 of the vSphere integration, run:

Linux:
```
sudo -u dd-agent -- datadog-agent integration install datadog-vsphere==3.6.0
```
Windows:
```
"C:\Program Files\Datadog\Datadog Agent\embedded\agent.exe" integration install datadog-vsphere==3.6.0
```

The command installs the Python package of the integration and copies the configuration files (`conf.yaml.example`, `conf.yaml.default`, `auto_conf.yaml`) to the `conf.d` directory, overwriting the existing ones. The same thing is done during a full Agent upgrade. If a failure occurs while copying of the files, the command exits with a failure, but the version of the integration you specified still gets installed.

After upgrading, restart your Agent to begin using the newly installed integration.

This command is designed specifically to allow users to upgrade an individual integration to get a new feature or bugfix as soon as it is available, without needing to wait for the next release of the Agent. **Note**: It is still recommended to upgrade the Agent when possible, as it always ships the latest version of every integration at the time of the release.

Upon Agent upgrade, every integration that you individually upgraded using the command gets overwritten by the integration shipped within the Agent.

#### Configuration management tools

Configuration management tools can leverage this command to deploy the version of an integration across your entire infrastructure.

### Remove

To remove an integration, use the `datadog-agent integration remove` command. The syntax for this command is `datadog-agent integration remove <integration_package_name>` where `<integration_package_name>` is the name of the integration prefixed with `datadog-`.

For example, to remove the vSphere integration, run:

Linux:
```
sudo -u dd-agent -- datadog-agent integration remove datadog-vsphere
```
Windows:
```
"C:\Program Files\Datadog\Datadog Agent\embedded\agent.exe" integration remove datadog-vsphere
```

Removing an integration does not remove the corresponding configuration folder in the `conf.d` directory.

### Show

To get information, such as the version, about an installed integration, use the `datadog-agent integration show` command. The syntax for this command is `datadog-agent integration show <integration_package_name>` where `<integration_package_name>` is the name of the integration prefixed with `datadog-`.

For example, to show information on the vSphere integration, run:

Linux:
```
sudo -u dd-agent -- datadog-agent integration show datadog-vsphere
```
Windows:
```
"C:\Program Files\Datadog\Datadog Agent\embedded\agent.exe" integration show datadog-vsphere
```

### Freeze

To list all the Python packages installed in the Agent's Python environment, use the `datadog-agent integration freeze` command. This lists all the Datadog integrations (packages starting with `datadog-`) and the Python dependencies required to run the integrations.

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
