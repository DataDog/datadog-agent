#!/bin/bash
# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# Datadog Agent install script for macOS.
set -e
logfile=ddagent-install.log
dmg_file=/tmp/datadog-agent.dmg
dmg_url="https://s3.amazonaws.com/dd-agent/datadog-agent-6.0.0-beta.9-1-with-install-script-fix.dmg"

# Root user detection
if [ $(echo "$UID") = "0" ]; then
    sudo_cmd=''
else
    sudo_cmd='sudo'
fi

# get real user (in case of sudo)
real_user=`logname`
export TMPDIR=`sudo -u $real_user getconf DARWIN_USER_TEMP_DIR`
cmd_real_user="sudo -Eu $real_user"

# In order to install with the right user
rm -f /tmp/datadog-install-user
echo $real_user > /tmp/datadog-install-user

function on_error() {
    printf "\033[31m$ERROR_MESSAGE
It looks like you hit an issue when trying to install the Agent.

Troubleshooting and basic usage information for the Agent are available at:

    http://docs.datadoghq.com/guides/basic_agent_usage/

If you're still having problems, please send an email to support@datadoghq.com
with the contents of ddagent-install.log and we'll do our very best to help you
solve your problem.\n\033[0m\n"
}
trap on_error ERR

cmd_agent="$cmd_real_user /opt/datadog-agent/bin/agent/agent"

cmd_launchctl="$cmd_real_user launchctl"

function new_config() {
    if [ -n "$DD_API_KEY" ]; then
        apikey=$DD_API_KEY
    fi

    if [ ! $apikey ]; then
        printf "\033[31mAPI key not available in DD_API_KEY environment variable.\033[0m\n"
        exit 1;
    fi
    # Check for vanilla OS X sed or GNU sed
    i_cmd="-i ''"
    if [ $(sed --version 2>/dev/null | grep -c "GNU") -ne 0 ]; then i_cmd="-i"; fi
    $sudo_cmd sh -c "sed $i_cmd 's/api_key:.*/api_key: $apikey/' \"/opt/datadog-agent/etc/datadog.yaml\""
    $sudo_cmd chown $real_user:admin "/opt/datadog-agent/etc/datadog.yaml"
    $sudo_cmd chmod 640 /opt/datadog-agent/etc/datadog.yaml
}

function import_config() {
    printf "\033[34m\n* Converting old datadog.conf file to new datadog.yaml format\n\033[0m\n"
    $cmd_agent import /opt/datadog-agent/etc /opt/datadog-agent/etc -f
}

# # Install the agent
printf "\033[34m\n* Downloading datadog-agent\n\033[0m"
rm -f $dmg_file
curl --progress-bar $dmg_url > $dmg_file
printf "\033[34m\n* Installing datadog-agent, you might be asked for your sudo password...\n\033[0m"
$sudo_cmd hdiutil detach "/Volumes/datadog_agent" >/dev/null 2>&1 || true
printf "\033[34m\n    - Mounting the DMG installer...\n\033[0m"
$sudo_cmd hdiutil attach "$dmg_file" -mountpoint "/Volumes/datadog_agent" >/dev/null
printf "\033[34m\n    - Unpacking and copying files (this usually takes about a minute) ...\n\033[0m"
cd / && $sudo_cmd /usr/sbin/installer -pkg `find "/Volumes/datadog_agent" -name \*.pkg 2>/dev/null` -target / >/dev/null
printf "\033[34m\n    - Unmounting the DMG installer ...\n\033[0m"
$sudo_cmd hdiutil detach "/Volumes/datadog_agent" >/dev/null

# Set the configuration
if egrep 'api_key:( APIKEY)?$' "/opt/datadog-agent/etc/datadog.yaml" > /dev/null 2>&1; then
    if [ ! -f /opt/datadog-agent/etc/datadog.conf ]; then
        new_config
    else
        import_config
    fi
    printf "\n\033[34m* Restarting the Agent...\n\033[0m\n"
    $cmd_launchctl stop com.datadoghq.agent
    $cmd_launchctl start com.datadoghq.agent
else
    printf "\033[34m\n* Keeping old datadog.yaml configuration file\n\033[0m\n"
fi

# Starting the app
$cmd_real_user open -a 'Datadog Agent.app'

# Agent works, echo some instructions and exit
printf "\033[32m

Your Agent is running properly. It will continue to run in the
background and submit metrics to Datadog.

You can check the agent status using the \"datadog-agent status\" command
or by openning the webui using the \"datadog-agent launch-gui\" command.

If you ever want to stop the Agent, please use the Datadog Agent App or
the launchctl command. It will start automatically at login.

\033[0m"
