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

# Colours
readonly C_NOC="\\033[0m"    # No colour
readonly C_RED="\\033[0;31m" # Red
readonly C_GRN="\\033[0;32m" # Green
readonly C_BLU="\\033[0;34m" # Blue
readonly C_PUR="\\033[0;35m" # Purple
readonly C_CYA="\\033[0;36m" # Cyan
readonly C_WHI="\\033[0;37m" # White

### Helper functions
print_red () { local i; for i in "$@"; do echo -e "${C_RED}${i}${C_NOC}"; done }
print_grn () { local i; for i in "$@"; do echo -e "${C_GRN}${i}${C_NOC}"; done }
print_blu () { local i; for i in "$@"; do echo -e "${C_BLU}${i}${C_NOC}"; done }
print_pur () { local i; for i in "$@"; do echo -e "${C_PUR}${i}${C_NOC}"; done }
print_cya () { local i; for i in "$@"; do echo -e "${C_CYA}${i}${C_NOC}"; done }
print_whi () { local i; for i in "$@"; do echo -e "${C_WHI}${i}${C_NOC}"; done }

function on_error() {
    print_red "$ERROR_MESSAGE
It looks like you hit an issue when trying to install the StackState Agent v2.

Basic information about the Agent are available at:

    https://docs.stackstate.com/integrations/agent/

If you're still having problems, please send an email to info@stackstate.com
with the contents of $logfile and we'll do our very best to help you
solve your problem.\n"
}
trap on_error ERR

if [ -n "$STS_API_KEY" ]; then
    api_key=$STS_API_KEY
fi

if [ -n "$STS_SITE" ]; then
    site="$STS_SITE"
fi

if [ -n "$STS_URL" ]; then
    sts_url=$STS_URL
fi

no_start=
if [ -n "$STS_INSTALL_ONLY" ]; then
    no_start=true
fi

no_repo=
if [ ! -z "$STS_INSTALL_NO_REPO" ]; then
    no_repo=true
fi

if [ -n "$STS_HOSTNAME" ]; then
    hostname=$STS_HOSTNAME
fi

# comma-separated list of tags
default_host_tags="os:linux"
if [ -n "$HOST_TAGS" ]; then
    host_tags="$default_host_tags,$HOST_TAGS"
else
    host_tags=$default_host_tags
fi

if [ -n "$CODE_NAME" ]; then
    code_name=$CODE_NAME
else
    code_name="stable"
fi

if [ -z "$DEBIAN_REPO"]; then
    # for offline script remember default production repo address
    DEBIAN_REPO="https://stackstate-agent-2.s3.amazonaws.com"
fi

if [ -z "$YUM_REPO"]; then
    # for offline script remember default production repo address
    YUM_REPO="https://stackstate-agent-2-rpm.s3.amazonaws.com"
fi

if [ -n "$SKIP_SSL_VALIDATION" ]; then
    skip_ssl_validation=$SKIP_SSL_VALIDATION
fi

if [ ! $api_key ]; then
    print_red "API key not available in STS_API_KEY environment variable.\n"
    exit 1
fi

if [ ! $sts_url ]; then
    print_red "StackState url not available in STS_URL environment variable.\n"
    exit 1
fi

INSTALL_MODE="REPO"
if [ ! -z "$1" ]; then
    if test -f "$1"; then
        print_blu "Trying to install from local package $1"
        INSTALL_MODE="FILE"
        LOCAL_PKG_NAME="$1"
    fi
fi

# OS/Distro Detection
# Try lsb_release, fallback with /etc/issue then uname command
KNOWN_DISTRIBUTION="(Debian|Ubuntu|RedHat|CentOS|openSUSE|Amazon|Arista|SUSE)"
DISTRIBUTION=$(lsb_release -d 2>/dev/null | grep -Eo $KNOWN_DISTRIBUTION || grep -Eo $KNOWN_DISTRIBUTION /etc/issue 2>/dev/null || grep -Eo $KNOWN_DISTRIBUTION /etc/Eos-release 2>/dev/null || grep -m1 -Eo $KNOWN_DISTRIBUTION /etc/os-release 2>/dev/null || uname -s)

if [ $DISTRIBUTION = "Darwin" ]; then
    print_red "This script does not support installing on the Mac.

Please use the 1-step script available at https://app.datadoghq.com/account/settings#agent/mac."
    exit 1

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
if [ $OS = "RedHat" ]; then
    if [ -z "$no_repo" ]; then
    print_blu "* Installing YUM sources for StackState\n"
    $sudo_cmd sh -c "echo -e '[stackstate]\nname = StackState\nbaseurl = $YUM_REPO/$code_name/\nenabled=1\ngpgcheck=1\npriority=1\ngpgkey=$YUM_REPO/public.key' > /etc/yum.repos.d/stackstate.repo"
    fi
    print_blu "* Installing the StackState Agent v2 package\n"
    $sudo_cmd yum -y clean metadata
    if [ $INSTALL_MODE = "REPO" ]; then
        $sudo_cmd yum -y --disablerepo='*' --enablerepo='stackstate' install $PKG_NAME || $sudo_cmd yum -y install $PKG_NAME
    else
        $sudo_cmd yum -y localinstall $LOCAL_PKG_NAME
    fi

elif [ $OS = "Debian" ]; then
    print_blu "* Installing apt-transport-https\n"
    $sudo_cmd apt-get update || print_red "'apt-get update' failed, the script will not install the latest version of apt-transport-https."
    $sudo_cmd apt-get install -y apt-transport-https || print_red "> 'apt-transport-https' was not installed"
    # Only install dirmngr if it's available in the cache
    # it may not be available on Ubuntu <= 14.04 but it's not required there
    cache_output=$(apt-cache search dirmngr)
    if [ ! -z "$cache_output" ]; then
        $sudo_cmd apt-get install -y dirmngr
    fi

    print_blu "* Configuring APT package sources for StackState\n"
    $sudo_cmd sh -c "echo 'deb $DEBIAN_REPO $code_name main' > /etc/apt/sources.list.d/stackstate.list"
    if [[ $INSTALL_MODE == "REPO" ]]; then
        $sudo_cmd apt-key adv --recv-keys --keyserver hkp://keyserver.ubuntu.com:80 B3CC4376
    else
        $sudo_cmd apt-key adv --recv-keys --keyserver hkp://keyserver.ubuntu.com:80 B3CC4376 || print_red "> Failed to install apt repo key (no internet connection?). Please install separately for further repo updates"
    fi

    print_blu "* Installing the StackState Agent v2 package\n"
    ERROR_MESSAGE="ERROR
Failed to update the sources after adding the StackState repository.
This may be due to any of the configured APT sources failing -
see the logs above to determine the cause.
If the failing repository is StackState, please contact StackState support.
*****
"

    if [[ $INSTALL_MODE == "REPO" ]]; then
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
        print_blu "* ($INSTALL_MODE) Installing local deb package $LOCAL_PKG_NAME \n"
        $sudo_cmd dpkg -i $LOCAL_PKG_NAME
    fi
    if [ ! -z "$no_repo" ]; then
        $sudo_cmd rm -f /etc/apt/sources.list.d/stackstate.list
    fi
else
    print_red "Your OS or distribution is not supported yet.\n"
    exit 1
fi

# Set the configuration
if [ ! -e $CONF ]; then
    $sudo_cmd cp $CONF.example $CONF
fi
if [ $api_key ]; then
    print_blu "* Adding your API key to the Agent configuration: $CONF\n"
    $sudo_cmd sh -c "sed -i 's/api_key:.*/api_key: $api_key/' $CONF"
fi
if [ $sts_url ]; then
    sts_url_esc=$(sed 's/[/.&]/\\&/g' <<<"$sts_url")
    print_blu "* Adding StackState url to the Agent configuration: $CONF\n"
    $sudo_cmd sh -c "sed -i 's/sts_url:.*/sts_url: $sts_url_esc/' $CONF"
fi
if [ $hostname ]; then
    print_blu "* Adding your STS_HOSTNAME to the Agent configuration: $CONF\n"
    $sudo_cmd sh -c "sed -i 's/# hostname:.*/hostname: $hostname/' $CONF"
fi
if [ $host_tags ]; then
    print_blu "* Adding your HOST TAGS to the Agent configuration: $CONF\n"
    formatted_host_tags="['"$(echo "$host_tags" | sed "s/,/','/g")"']" # format `env:prod,foo:bar` to yaml-compliant `['env:prod','foo:bar']`
    $sudo_cmd sh -c "sed -i \"s/# tags:.*/tags: "$formatted_host_tags"/\" $CONF"
fi
if [ $skip_ssl_validation ]; then
    print_blu "* Skipping SSL validation in the Agent configuration: $CONF\n"
    $sudo_cmd sh -c "sed -i 's/# skip_ssl_validation:.*/skip_ssl_validation: $skip_ssl_validation/' $CONF"
fi

function version_gt() {
    test "$(printf '%s\n' "$@" | sort -V | head -n 1)" != "$1"
}

#Minimum kernel version required for network tracer https://github.com/StackVista/tcptracer-bpf/blob/master/pkg/tracer/common/common_linux.go#L28
min_required_kernel="4.3.0"
current_kernel=$(uname -r)
if version_gt $min_required_kernel $current_kernel; then
    print_cya "* The network tracer does not support your kernel version (min required $min_required_kernel), disabling it\n"
    $sudo_cmd sh -c "sed -i \"s/network_tracing_enabled:.*/network_tracing_enabled: 'false'/\" $CONF"
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
elif [[ -d /etc/rc.d/ || -d /etc/init.d/ ]]; then
    # Use sysv-init
    restart_cmd="$sudo_cmd service $PKG_NAME restart"
    stop_instructions="$sudo_cmd service $PKG_NAME stop"
    start_instructions="$sudo_cmd service $PKG_NAME start"
fi

if [ $no_start ]; then
    print_blu "
* STS_INSTALL_ONLY environment variable set: the newly installed version of the agent
will not be started. You will have to do it manually using the following
command:

    $restart_cmd
\n"
    exit
fi

print_blu "* Starting the Agent...\n"
eval $restart_cmd

# Metrics are submitted, echo some instructions and exit
print_grn "
Your Agent is running and functioning properly. It will continue to run in the
background and submit metrics to StackState.

If you ever want to stop the Agent, run:

    $stop_instructions

And to run it again run:

    $start_instructions
\n"
