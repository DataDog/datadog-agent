# Upgrade to Agent 6

* [Linux](#linux)
* [Windows](#windows)
* [MacOS](#macos)

## Linux

### One-step install

A script is available to automatically install or upgrade the new Agent. It will
set up the repos and install the package for you; in case of upgrade, the import
tool will also search for an existing `datadog.conf` from a prior version and will
convert Agent and checks configurations according to the new file format and
filesystem layout.

#### To Upgrade

In case you have an Agent version 5.17 or later and you want to import the
existing configuration:

```shell
 DD_UPGRADE=true bash -c "$(curl -L https://raw.githubusercontent.com/DataDog/datadog-agent/master/cmd/agent/install_script.sh)"
```

**Note:** the import process won't automatically move custom checks, this is by
design since we cannot guarantee full backwards compatibility out of the box.

#### To Install Fresh

In case you want to install on a clean box (or have an existing agent 5 install
from which you do not wish to import the configuration) you have to provide an
api key:

```shell
 DD_API_KEY=YOUR_API_KEY bash -c "$(curl -L https://raw.githubusercontent.com/DataDog/datadog-agent/master/cmd/agent/install_script.sh)"
```

### Manual install

#### Manual install: Debian Flavored Systems

##### Set up apt so it can download through https

```shell
sudo apt-get update
sudo apt-get install apt-transport-https
```

##### Add the beta repo to your system and import the datadog gpg key

```shell
echo 'deb https://apt.datadoghq.com/ beta main' | sudo tee /etc/apt/sources.list.d/datadog-beta.list
sudo apt-key adv --recv-keys --keyserver hkp://keyserver.ubuntu.com:80 C7A7DA52
```

##### Update Apt and Install the agent

```shell
sudo apt-get update
sudo apt-get install datadog-agent
```


#### Red Hat flavored systems

##### Set up Datadog's Yum repo on your system

```
[datadog-beta]
name = Beta, Datadog, Inc.
baseurl = https://yum.datadoghq.com/beta/x86_64/
enabled=1
gpgcheck=1
priority=1
gpgkey=https://yum.datadoghq.com/DATADOG_RPM_KEY.public
       https://yum.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public
```

You can use this command to do it directly:

```shell
# Red Hat
echo -e '[datadog-beta]\nname = Beta, Datadog, Inc.\nbaseurl = https://yum.datadoghq.com/beta/x86_64/\nenabled=1\ngpgcheck=1\npriority=1\ngpgkey=https://yum.datadoghq.com/DATADOG_RPM_KEY.public\n       https://yum.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public' | sudo tee /etc/yum.repos.d/datadog-beta.repo
```

##### Update your local yum cache and install/update the agent

```shell
sudo yum clean expire-cache
sudo yum install datadog-agent
```

#### Import existing configuration (optional)

If you ran the `install_script.sh` all agent and checks configuration should be already imported.

If you didn't you can run manually the import command:

```shell
/opt/datadog-agent/bin/agent/agent import /etc/dd-agent /etc/datadog-agent
```

As you'll see the agent 6 promotes a new directory structure with subfolders per check. This allows the regular configuration and the auto-configuration to sit next to each other.

#### Enable desired custom checks (optional)

Since we cannot guarantee all your custom checks will work on Agent 6, we'll let you enable
these manually. Just copy them over to the `additional_checksd` location (defaults to
`/etc/datadog-agent/checks.d/` for Agent 6):

```shell
sudo -u dd-agent -- cp /etc/dd-agent/checks.d/<check>.py /etc/datadog-agent/checks.d/
```

**Note:** custom checks now have a *lower* precedence than the checks bundled by default with the Agent.
This will affect your custom checks if they have the same name as a check in [integrations-core][integrations-core].
Please read the [relevant section of the changes document][changes-custom-check] for more information.

#### Restart the agent

```shell
# Systemd
sudo systemctl restart datadog-agent
# Upstart
sudo restart datadog-agent
```

## Windows

Download the latest version available [from here](https://github.com/DataDog/datadog-agent/releases)
and run the installation package.


## MacOS

You can either download the DMG package and install it manually, or use the one-line install script.

### Manual installation

1. Download the DMG package of the latest beta version, please use the latest macOS release listed on the [release page](https://github.com/DataDog/datadog-agent/releases) of the repo
2. Install the DMG package
3. Add your api key to `/opt/datadog-agent/etc/datadog.yaml`

You can then start the Datadog Agent app (once started, you should see it in the system tray), and manage the Agent from there. The Agent6 also ships a web-based GUI to edit the Agent configuration files and much more, refer to the [changes and deprecations document][changes] document for more information.

Unlike on Linux, the configuration path hasn't changed and remains in `~/.datadog-agent` (which links to `/opt/datadog-agent/etc`).

### Install script

#### To Upgrade

In case you have an Agent version 5 and you want to import the existing
configuration:

```shell
  DD_UPGRADE=true bash -c "$(curl -L https://raw.githubusercontent.com/DataDog/datadog-agent/master/cmd/agent/install_mac_os.sh)"
```

#### To Install Fresh

In case you want to install on a clean box (or have an existing agent 5 install
from which you do not wish to import the configuration) you have to provide an
api key:

```shell
 DD_API_KEY=YOUR_API_KEY bash -c "$(curl -L https://raw.githubusercontent.com/DataDog/datadog-agent/master/cmd/agent/install_mac_os.sh)"
```

[changes]: changes.md
[integrations-core]: https://github.com/DataDog/integrations-core
[changes-custom-check]: changes.md#custom-check-precedence
