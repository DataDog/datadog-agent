#!/bin/bash
# (C) Datadog, Inc. 2010-2016
# (C) StackState
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)
# StackState Agent installation script: install and set up the Agent on supported Linux distributions
# using the package manager and StackState repositories.

set -e

PKG_NAME="stackstate-agent"
PKG_USER="stackstate-agent"
ETCDIR="/etc/stackstate-agent"
CONF="$ETCDIR/stackstate.yaml"

logfile="$PKG_NAME-install.log"

if [ $(command -v curl) ]; then
    dl_cmd="curl -f"
else
    dl_cmd="wget --quiet"
fi

# Set up a named pipe for logging
npipe=/tmp/$$.tmp
mknod $npipe p

# Log all output to a log for error checking
tee <$npipe $logfile &
exec 1>&-
exec 1>$npipe 2>&1
trap "rm -f $npipe" EXIT


function on_error() {
    printf "\033[31m$ERROR_MESSAGE
It looks like you hit an issue when trying to install the StackState Agent v2.

Troubleshooting and basic usage information for the Agent are available at:

    https://docs.stackstate.com/guides/basic_agent_usage/

If you're still having problems, please send an email to info@stackstate.com
with the contents of $logfile and we'll do our very best to help you
solve your problem.\n\033[0m\n"
}
trap on_error ERR

if [ -n "$STS_HOSTNAME" ]; then
    sts_hostname=$STS_HOSTNAME
fi

if [ -n "$STS_API_KEY" ]; then
    api_key=$STS_API_KEY
fi

no_start=
if [ -n "$STS_INSTALL_ONLY" ]; then
    no_start=true
fi

# comma-separated list of tags
if [ -n "$STS_HOST_TAGS" ]; then
    host_tags=$STS_HOST_TAGS
fi

if [ -n "$STS_URL" ]; then
    sts_url=$STS_URL
fi

if [ -n "$CODE_NAME" ]; then
    code_name=$CODE_NAME
else
    code_name="stable"
fi

if [ ! $api_key ]; then
    printf "\033[31mAPI key not available in STS_API_KEY environment variable.\033[0m\n"
    exit 1;
fi

if [ ! $sts_url ]; then
    printf "\033[31mStackState url not available in STS_URL environment variable.\033[0m\n"
    exit 1;
fi

# OS/Distro Detection
# Try lsb_release, fallback with /etc/issue then uname command
KNOWN_DISTRIBUTION="(Debian|Ubuntu|RedHat|CentOS|openSUSE|Amazon|Arista|SUSE)"
DISTRIBUTION=$(lsb_release -d 2>/dev/null | grep -Eo $KNOWN_DISTRIBUTION  || grep -Eo $KNOWN_DISTRIBUTION /etc/issue 2>/dev/null || grep -Eo $KNOWN_DISTRIBUTION /etc/Eos-release 2>/dev/null || grep -m1 -Eo $KNOWN_DISTRIBUTION /etc/os-release 2>/dev/null || uname -s)

if [ $DISTRIBUTION = "Darwin" ]; then
    printf "\033[31mThis script does not support installing on the Mac.

Please use the 1-step script available at https://app.datadoghq.com/account/settings#agent/mac.\033[0m\n"
    exit 1;

elif [ -f /etc/debian_version -o "$DISTRIBUTION" == "Debian" -o "$DISTRIBUTION" == "Ubuntu" ]; then
    OS="Debian"
elif [ -f /etc/redhat-release -o "$DISTRIBUTION" == "RedHat" -o "$DISTRIBUTION" == "CentOS" -o "$DISTRIBUTION" == "Amazon" ]; then
    OS="RedHat"
# Some newer distros like Amazon may not have a redhat-release file
elif [ -f /etc/system-release -o "$DISTRIBUTION" == "Amazon" ]; then
    OS="RedHat"
# Arista is based off of Fedora14/18 but do not have /etc/redhat-release
elif [ -f /etc/Eos-release -o "$DISTRIBUTION" == "Arista" ]; then
    OS="RedHat"
# openSUSE and SUSE use /etc/SuSE-release or /etc/os-release
elif [ -f /etc/SuSE-release -o "$DISTRIBUTION" == "SUSE" -o "$DISTRIBUTION" == "openSUSE" ]; then
    OS="SUSE"
fi

# Root user detection
if [ $(echo "$UID") = "0" ]; then
    sudo_cmd=''
else
    sudo_cmd='sudo'
fi

# Install the necessary package sources
if [ $OS = "Debian" ]; then
    printf "\033[34m\n* Installing apt-transport-https\n\033[0m\n"
    $sudo_cmd apt-get update || printf "\033[31m'apt-get update' failed, the script will not install the latest version of apt-transport-https.\033[0m\n"
    $sudo_cmd apt-get install -y apt-transport-https
    # Only install dirmngr if it's available in the cache
    # it may not be available on Ubuntu <= 14.04 but it's not required there
    cache_output=`apt-cache search dirmngr`
    if [ ! -z "$cache_output" ]; then
      $sudo_cmd apt-get install -y dirmngr
    fi
    printf "\033[34m\n* Configuring APT package sources for StackState\n\033[0m\n"
    $sudo_cmd sh -c "echo 'deb $DEBIAN_REPO $code_name main' > /etc/apt/sources.list.d/stackstate.list"
    $sudo_cmd apt-key adv --recv-keys --keyserver hkp://keyserver.ubuntu.com:80 B3CC4376

    printf "\033[34m\n* Installing the StackState Agent v2 package\n\033[0m\n"
    ERROR_MESSAGE="ERROR
Failed to update the sources after adding the StackState repository.
This may be due to any of the configured APT sources failing -
see the logs above to determine the cause.
If the failing repository is StackState, please contact StackState support.
*****
"
    $sudo_cmd apt-get update -o Dir::Etc::sourcelist="sources.list.d/stackstate.list" -o Dir::Etc::sourceparts="-" -o APT::Get::List-Cleanup="0"
    ERROR_MESSAGE="ERROR
Failed to install the StackState package, sometimes it may be
due to another APT source failing. See the logs above to
determine the cause.
If the cause is unclear, please contact StackState support.
*****
"
    $sudo_cmd apt-get install -y --force-yes $PKG_NAME
    ERROR_MESSAGE=""
else
    printf "\033[31mYour OS or distribution is not supported yet.\033[0m\n"
    exit 1;
fi

# Set the configuration
if [ ! -e $CONF ]; then
    $sudo_cmd cp $CONF.example $CONF
fi
if [ $api_key ]; then
    printf "\033[34m\n* Adding your API key to the Agent configuration: $CONF\n\033[0m\n"
    $sudo_cmd sh -c "sed -i 's/api_key:.*/api_key: $api_key/' $CONF"
fi
if [ $sts_url ]; then
    $sudo_cmd sh -c "sed -i 's/sts_url:.*/sts_url: $sts_url/' $CONF"
fi
if [ $sts_hostname ]; then
    printf "\033[34m\n* Adding your HOSTNAME to the Agent configuration: $CONF\n\033[0m\n"
    $sudo_cmd sh -c "sed -i 's/# hostname:.*/hostname: $sts_hostname/' $CONF"
fi
if [ $host_tags ]; then
    printf "\033[34m\n* Adding your HOST TAGS to the Agent configuration: $CONF\n\033[0m\n"
    formatted_host_tags="['"$( echo "$host_tags" | sed "s/,/','/g" )"']"  # format `env:prod,foo:bar` to yaml-compliant `['env:prod','foo:bar']`
    $sudo_cmd sh -c "sed -i \"s/# tags:.*/tags: "$formatted_host_tags"/\" $CONF"
fi
$sudo_cmd chown $PKG_USER:$PKG_USER $CONF
$sudo_cmd chmod 640 $CONF


# Use systemd by default
restart_cmd="$sudo_cmd systemctl restart $PKG_NAME.service"
stop_instructions="$sudo_cmd systemctl stop $PKG_NAME"
start_instructions="$sudo_cmd systemctl start $PKG_NAME"

# Try to detect Upstart, this works most of the times but still a best effort
if /sbin/init --version 2>&1 | grep -q upstart; then
    restart_cmd="$sudo_cmd start $PKG_NAME"
    stop_instructions="$sudo_cmd stop $PKG_NAME"
    start_instructions="$sudo_cmd start $PKG_NAME"
fi

if [ $no_start ]; then
    printf "\033[34m
* STS_INSTALL_ONLY environment variable set: the newly installed version of the agent
will not be started. You will have to do it manually using the following
command:

    $restart_cmd

\033[0m\n"
    exit
fi

printf "\033[34m* Starting the Agent...\n\033[0m\n"
eval $restart_cmd


# Metrics are submitted, echo some instructions and exit
printf "\033[32m
Your Agent is running and functioning properly. It will continue to run in the
background and submit metrics to StackState.

If you ever want to stop the Agent, run:

    $stop_instructions

And to run it again run:

    $start_instructions

\033[0m"
