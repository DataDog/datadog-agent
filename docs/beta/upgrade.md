# Upgrade to Agent 6

## Linux

### Debian Flavored Systems

### Set up apt so it can download through https

```shell
sudo apt-get update
sudo apt-get install apt-transport-https
```

### Add the beta repo to your system and import the datadog gpg key

```shell
echo 'deb https://apt.datadoghq.com/ beta main' | sudo tee /etc/apt/sources.list.d/datadog-beta.list
sudo apt-key adv --recv-keys --keyserver hkp://keyserver.ubuntu.com:80 C7A7DA52
```

### Update Apt and Install the agent
```shell
sudo apt-get update
sudo apt-get install datadog-agent
```

### Red Hat Flavored Systems

```shell
# Red Hat
echo -e '[datadog]\nname = Datadog, Inc.\nbaseurl = https://yum.datadoghq.com/beta/$ARCHI/\nenabled=1\ngpgcheck=1\npriority=1\ngpgkey=$PROTOCOL://yum.datadoghq.com/DATADOG_RPM_KEY.public\n       $PROTOCOL://yum.datadoghq.com/DATADOG_RPM_KEY_E09422B3.public' | sudo tee /etc/yum.repos.d/datadog-beta.repo
```

## Windows
