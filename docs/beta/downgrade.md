# Downgrade back to Agent 5

This guide assumes you upgraded to the beta using our [upgrade guide][upgrade-guide]

We have been careful to keep the legacy configurations in place to ease the the
downgrade process should you decide you do not wish to continue trying the beta.

## Linux

### Debian Flavored Systems

#### Set up apt so it can download through https
```shell
sudo apt-get update
sudo apt-get install apt-transport-https
```

#### Remove Beta Repo and Ensure the stable repo is present
```shell
sudo rm /etc/apt/sources.list.d/datadog-beta.list
[ ! -f /etc/apt/sources.list.d/datadog.list ] &&  echo 'deb https://apt.datadoghq.com/ stable main' | sudo tee /etc/apt/sources.list.d/datadog.list
sudo apt-key adv --recv-keys --keyserver hkp://keyserver.ubuntu.com:80 C7A7DA52
```

#### Update apt and downgrade the agent
```shell
sudo apt-get update
sudo apt-get remove datadog-agent
sudo apt-get install datadog-agent
```

#### Back-sync configurations and AutoDiscovery templates (optional)
If you have made any changes to your configurations or templates, you might want
to sync these back for agent 5.

```shell
sudo -u dd-agent -- cp /etc/datadog-agent/conf.d/*.yaml /etc/dd-agent/conf.d/
sudo -u dd-agent -- cp /etc/datadog-agent/conf.d/auto_conf/*.yaml /etc/dd-agent/conf.d/auto_conf/
```

#### Back-sync custom checks (optional)
If you made any changes or added any new custom checks while testing Agent 6 you might want
to enable them back on Agent 5. Note: you only need to copy back checks you changed.
```shell
sudo -u dd-agent -- cp /etc/datadog-agent/checks.d/<check>.py /etc/dd-agent/checks.d/
```

#### Restart the agent
```shell
# Systemd
sudo systemctl restart datadog-agent
# Upstart
sudo restart datadog-agent
```

#### Clean out /etc/datadog-agent (optional)
```shell
sudo -u dd-agent -- rm -rf /etc/datadog-agent/
```

### Red Hat Flavored Systems

#### Remove the Beta Yum repo from your system:
```shell
rm /etc/yum.repos.d/datadog-beta.repo
[ ! -f /etc/yum.repos.d/datadog.repo ] && echo -e '[datadog]\nname = Datadog, Inc.\nbaseurl = https://yum.datadoghq.com/rpm/x86_64/\nenabled=1\ngpgcheck=1\npriority=1\ngpgkey=https://yum.datadoghq.com/DATADOG_RPM_KEY.public\n       https://yum.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public' | sudo tee /etc/yum.repos.d/datadog.repo
```

#### Update your local yum cache and downgrade the agent
```shell
sudo yum clean expire-cache
sudo yum check-update
sudo yum remove datadog-agent
sudo yum install datadog-agent
```

#### Back-sync configurations and AutoDiscovery templates (optional)
If you have made any changes to your configurations or templates, you might want
to sync these back for agent 5.
```shell
sudo -u dd-agent -- cp /etc/datadog-agent/conf.d/*.yaml /etc/dd-agent/conf.d/
sudo -u dd-agent -- cp /etc/datadog-agent/conf.d/auto_conf/*.yaml /etc/dd-agent/conf.d/auto_conf/
```

#### Back-sync custom checks (optional)
If you made any changes or added any new custom checks while testing Agent 6 you might want
to enable them back on Agent 5. Note: you only need to copy back checks you changed.
```shell
sudo -u dd-agent -- cp /etc/datadog-agent/checks.d/<check>.py /etc/dd-agent/checks.d/
```

#### Restart the agent
```shell
# Systemd
sudo systemctl restart datadog-agent
# Upstart
sudo restart datadog-agent
```

#### Clean out /etc/datadog-agent (optional)
```shell
sudo -u dd-agent -- rm -rf /etc/datadog-agent/
```

## Windows

### Download the [Datadog Installer](https://s3.amazonaws.com/ddagent-windows-test/ddagent-cli-latest.msi)

### In a cmd.exe shell in the directory you downloaded the installer, run:

```shell
msiexec /qn /i ddagent-cli-beta-latest.msi APIKEY="your_api_key" HOSTNAME="my_hostname" TAGS="mytag1,mytag2"
```

[upgrade-guide]: upgrade.md
