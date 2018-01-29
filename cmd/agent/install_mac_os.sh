#!/bin/bash
# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)

# Datadog Agent install script for macOS.
set -e
logfile=ddagent-install.log
dmg_file=/tmp/datadog-agent.dmg
# FIXME: For now the script just installs the local DMG file at /tmp/datadog-agent.dmg
# Uncomment and update the link below when we've settled on a bucket and dmg name
# dmg_url="https://s3.amazonaws.com/dd-agent/datadogagent.dmg"

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

if [ -n "$DD_API_KEY" ]; then
    apikey=$DD_API_KEY
fi

if [ ! $apikey ]; then
    printf "\033[31mAPI key not available in DD_API_KEY environment variable.\033[0m\n"
    exit 1;
fi

# FIXME: Actually download the datadog-agent DMG
# # Install the agent
# printf "\033[34m\n* Downloading datadog-agent\n\033[0m"
# rm -f $dmg_file
# curl $dmg_url > $dmg_file
printf "\033[34m\n* Installing datadog-agent, you might be asked for your sudo password...\n\033[0m"
$sudo_cmd hdiutil detach "/Volumes/datadog_agent" >/dev/null 2>&1 || true
printf "\033[34m\n    - Mounting the DMG installer...\n\033[0m"
$sudo_cmd hdiutil attach "$dmg_file" -mountpoint "/Volumes/datadog_agent" >/dev/null
printf "\033[34m\n    - Unpacking and copying files (this usually takes about a minute) ...\n\033[0m"
cd / && $sudo_cmd /usr/sbin/installer -pkg `find "/Volumes/datadog_agent" -name \*.pkg 2>/dev/null` -target / >/dev/null
printf "\033[34m\n    - Unmounting the DMG installer ...\n\033[0m"
$sudo_cmd hdiutil detach "/Volumes/datadog_agent" >/dev/null

# TODO: use import command to import datadog.conf, if datadog.yaml isn't present
# Set the configuration
if egrep 'api_key:( APIKEY)?$' "/opt/datadog-agent/etc/datadog.yaml" > /dev/null 2>&1; then
    printf "\033[34m\n* Adding your API key to the Agent configuration: datadog.yaml\n\033[0m\n"
    # Check for vanilla OS X sed or GNU sed
    i_cmd="-i ''"
    if [ $(sed --version 2>/dev/null | grep -c "GNU") -ne 0 ]; then i_cmd="-i"; fi
    $sudo_cmd sh -c "sed $i_cmd 's/api_key:.*/api_key: $apikey/' \"/opt/datadog-agent/etc/datadog.yaml\""
    $sudo_cmd chown $real_user:admin "/opt/datadog-agent/etc/datadog.yaml"
    $sudo_cmd chmod 640 /opt/datadog-agent/etc/datadog.yaml
    printf "\033[34m* Restarting the Agent...\n\033[0m\n"
    $cmd_real_user "/opt/datadog-agent/bin/datadog-agent" restart >/dev/null
else
    printf "\033[34m\n* Keeping old datadog.yaml configuration file\n\033[0m\n"
fi

# Starting the app
$cmd_real_user open -a 'Datadog Agent.app'

# TODO: either make the app start the datadog agent service, or start the service here.


## FIXME: the following commented out code was the Agent 5 way of checking that the forwarder
## was indeed sending payloads: unless there's a good way to port the check to Agent 6,
## we should remove it completely
# printf "\033[32m
# Your Agent has started up for the first time. We're currently verifying that
# data is being submitted. You should see your Agent show up in Datadog shortly
# at:

#     https://app.datadoghq.com/infrastructure\033[0m

# Waiting for metrics..."

# c=0
# while [ "$c" -lt "30" ]; do
#     sleep 1
#     echo -n "."
#     c=$(($c+1))
# done

# curl -f http://127.0.0.1:17123/status?threshold=0 > /dev/null 2>&1
# success=$?
# while [ "$success" -gt "0" ]; do
#     sleep 1
#     echo -n "."
#     curl -f http://127.0.0.1:17123/status?threshold=0 > /dev/null 2>&1
#     success=$?
# done

# Agent works, echo some instructions and exit
printf "\033[32m

Your Agent is running and functioning properly. It will continue to run in the
background and submit metrics to Datadog.

If you ever want to stop the Agent, please use the Datadog Agent App or
the launchctl command. It will start automatically at login.

\033[0m"
