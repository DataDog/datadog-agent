# Upgrade to Agent 6

## Linux

### Debian Flavored Systems

#### Set up apt so it can download through https

```shell
sudo apt-get update
sudo apt-get install apt-transport-https
```

#### Add the beta repo to your system and import the datadog gpg key

```shell
echo 'deb https://apt.datadoghq.com/ beta main' | sudo tee /etc/apt/sources.list.d/datadog-beta.list
sudo apt-key adv --recv-keys --keyserver hkp://keyserver.ubuntu.com:80 C7A7DA52
```

#### Update Apt and Install the agent
```shell
sudo apt-get update
sudo apt-get install datadog-agent
```

#### Additional custom AutoDiscovery templates (optional)
You may have added additional custom AutoDiscovery templates, if so you might want to import
them:

```shell
sudo -u dd-agent -- cp /etc/dd-agent/conf.d/auto_conf/*.yaml /etc/datadog-agent/conf.d/auto_conf/
```

#### Enable desired custom checks (optional)
Since we cannot guarantee all your custom checks will work on Agent 6, we'll let you enable
these manually. Just copy them over to the `additional_checksd` location (defaults to
`/etc/datadog-agent/checks.d/` for Agent 6:

```shell
sudo -u dd-agent -- cp /etc/dd-agent/checks.d/<check>.py /etc/datadog-agent/checks.d/
```

#### Restart the agent
```shell
# Systemd
sudo systemctl restart datadog-agent
# Upstart
sudo restart datadog-agent
```

### Red Hat Flavored Systems

#### Set up Datadog's Yum repo on your system:
```
[datadog]
name = Datadog, Inc.
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
echo -e '[datadog]\nname = Datadog, Inc.\nbaseurl = https://yum.datadoghq.com/beta/x86_64/\nenabled=1\ngpgcheck=1\npriority=1\ngpgkey=https://yum.datadoghq.com/DATADOG_RPM_KEY.public\n       https://yum.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public' | sudo tee /etc/yum.repos.d/datadog-beta.repo
```

#### Update your local yum cache and install/update the agent
```shell
sudo yum clean expire-cache
sudo yum check-update
sudo yum install datadog-agent
```

#### Additional custom AutoDiscovery templates (optional)
You may have added additional custom AutoDiscovery templates, if so you might want to import
them:

```shell
sudo -u dd-agent -- cp /etc/dd-agent/conf.d/auto_conf/*.yaml /etc/datadog-agent/conf.d/auto_conf/
```

#### Enable desired custom checks (optional)
Since we cannot guarantee all your custom checks will work on Agent 6, we'll let you enable
these manually. Just copy them over to the `additional_checksd` location (defaults to
`/etc/datadog-agent/checks.d/` for Agent 6:

```shell
sudo -u dd-agent -- cp /etc/dd-agent/checks.d/<check>.py /etc/datadog-agent/checks.d/
```

#### Restart the agent
```shell
# Systemd
sudo systemctl restart datadog-agent
# Upstart
sudo restart datadog-agent
```

## Windows

Coming soon.
