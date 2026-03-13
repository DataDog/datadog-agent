#!/usr/bin/env bash
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# Datadog Agent install script for macOS.
set -e
install_script_version=1.6.0

# Terminal color detection
# Colors are enabled only when outputting to a terminal (not when piped/redirected)
# This prevents ANSI escape codes from appearing in logs or breaking through SSH layers
if [ -t 1 ]; then
    RED='\033[31m'
    GREEN='\033[32m'
    YELLOW='\033[33m'
    BLUE='\033[34m'
    NC='\033[0m'  # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi
dmg_file=/tmp/datadog-agent.dmg
dmg_base_url="https://s3.amazonaws.com/dd-agent"
etc_dir=/opt/datadog-agent/etc
log_dir=/opt/datadog-agent/logs
run_dir=/opt/datadog-agent/run
service_name="com.datadoghq.agent"
systemwide_servicefile_name="/Library/LaunchDaemons/${service_name}.plist"
sysprobe_service_name=com.datadoghq.sysprobe
sysprobe_servicefile_name="/Library/LaunchDaemons/${sysprobe_service_name}.plist"

if [ -n "$DD_REPO_URL" ]; then
    dmg_base_url=$DD_REPO_URL
fi

upgrade=
if [ -n "$DD_UPGRADE" ]; then
    upgrade=$DD_UPGRADE
fi

# Root user detection
if [ "$(echo "$UID")" = "0" ]; then
    sudo_cmd=''
else
    sudo_cmd='sudo'
fi

apikey=
if [ -n "$DD_API_KEY" ]; then
    apikey=$DD_API_KEY
fi

site=
if [ -n "$DD_SITE" ]; then
    site=$DD_SITE
fi

agent_dist_channel=
if [ -n "$DD_AGENT_DIST_CHANNEL" ]; then
    agent_dist_channel="$DD_AGENT_DIST_CHANNEL"
fi

if [ -n "$DD_AGENT_MINOR_VERSION" ]; then
  # Examples:
  #  - 20   = defaults to highest patch version x.20.2
  #  - 20.0 = sets explicit patch version x.20.0
  # Note: Specifying an invalid minor version will terminate the script.
  agent_minor_version=${DD_AGENT_MINOR_VERSION}
  # Handle pre-release versions like "35.0~rc.5" -> "35.0" or "27.1~viper~conflict~fix" -> "27.1"
  clean_agent_minor_version=$(echo "${DD_AGENT_MINOR_VERSION}" | sed -E 's/-.*//g')
  # remove the patch version if the minor version includes it (eg: 33.1 -> 33)
  agent_minor_version_without_patch="${clean_agent_minor_version%.*}"
  if [ "$clean_agent_minor_version" != "$agent_minor_version_without_patch" ]; then
      agent_patch_version="${clean_agent_minor_version#*.}"
  fi
fi

arch=$(/usr/bin/uname -m)
curl_retries=(--retry 2)

function find_latest_patch_version_for() {
    major_minor="$1"
    patch_versions=$(curl "${curl_retries[@]}" "$dmg_base_url?prefix=datadog-agent-${major_minor}." 2>/dev/null | grep -Eo "datadog-agent-${major_minor}.[0-9]*-1(.$arch)?.dmg")
    if [ -z "$patch_versions" ]; then
        echo "-1"
    fi
    # first `cut` extracts patch version and `-1`, e.g. 2-1
    # second `cut` removes the `-1`, e.g. 2
    # then we sort numerically in reverse order
    # and finally take the first (== the latest) patch version
    latest_patch=$(echo "$patch_versions" | cut -d. -f3 | cut -d- -f1 | sort -rn | head -n 1)
    echo "$latest_patch"
}

function prepare_dmg_file() {
    dmg_file_to_prepare=$1
    $sudo_cmd rm -f "$dmg_file_to_prepare"
    $sudo_cmd touch "$dmg_file_to_prepare"
    $sudo_cmd chmod 600 "$dmg_file_to_prepare"

    if stat --help >/dev/null 2>&1; then # Handle differences between GNU and BSD stat command
        file_owner=$(stat -c %u "$dmg_file_to_prepare")
        file_permission=$(stat -c %a "$dmg_file_to_prepare")
        file_size=$(stat -c %s "$dmg_file_to_prepare")
    else
        file_owner=$(stat -f %u "$dmg_file_to_prepare")
        file_permission=$(stat -f %OLp "$dmg_file_to_prepare")
        file_size=$(stat -f %z "$dmg_file_to_prepare")
    fi

    if [[ "$file_owner" -ne 0 ]] || [[ "$file_permission" -ne 600 ]] || [[ "$file_size" -ne 0 ]]; then
        echo -e "${RED}Failed to prepare datadog-agent dmg file${NC}\n"
        exit 1;
    fi
}

macos_full_version=$(sw_vers -productVersion)
macos_major_version=$(echo "${macos_full_version}" | cut -d '.' -f 1)
macos_minor_version=$(echo "${macos_full_version}" | cut -d '.' -f 2)

agent_major_version=7
if [ -n "$DD_AGENT_MAJOR_VERSION" ]; then
  if [ "$DD_AGENT_MAJOR_VERSION" != "6" ] && [ "$DD_AGENT_MAJOR_VERSION" != "7" ]; then
    echo "DD_AGENT_MAJOR_VERSION must be either 6 or 7. Current value: $DD_AGENT_MAJOR_VERSION"
    exit 1;
  fi
  agent_major_version=$DD_AGENT_MAJOR_VERSION
else
  echo -e "${YELLOW}Warning: DD_AGENT_MAJOR_VERSION not set. Installing Agent version 7 by default.${NC}"
fi

dmg_version=
if [ "${macos_major_version}" -lt 10 ] || { [ "${macos_major_version}" -eq 10 ] && [ "${macos_minor_version}" -lt 12 ]; }; then
    echo -e "${RED}Datadog Agent doesn't support macOS < 10.12.${NC}\n"
    exit 1
elif [ "${macos_major_version}" -eq 10 ] && [ "${macos_minor_version}" -eq 12 ]; then
    if [ -n "${clean_agent_minor_version}" ]; then
        if [ "${agent_minor_version_without_patch}" -gt 34 ]; then
            echo -e "${RED}macOS 10.12 only supports Datadog Agent $agent_major_version up to $agent_major_version.34.${NC}\n"
            exit 1;
        fi
    else
        echo -e "${YELLOW}Warning: Agent ${agent_major_version}.34.0 is the last supported version for macOS 10.12. Selecting it for installation.${NC}"
        agent_minor_version_without_patch=34
        agent_patch_version=0
    fi
elif [ "${macos_major_version}" -eq 10 ] && [ "${macos_minor_version}" -eq 13 ]; then
    if [ -n "${clean_agent_minor_version}" ]; then
        if [ "${agent_minor_version_without_patch}" -gt 38 ]; then
            echo -e "${RED}macOS 10.13 only supports Datadog Agent $agent_major_version up to $agent_major_version.38.${NC}\n"
            exit 1;
        fi
    else
        echo -e "${YELLOW}Warning: Agent ${agent_major_version}.38.2 is the last supported version for macOS 10.13. Selecting it for installation.${NC}"
        agent_minor_version_without_patch=38
        agent_patch_version=2
    fi
else
    if [ "${agent_major_version}" -eq 6 ]; then
        echo -e "${RED}The latest Agent 6 is no longer built for for macOS $macos_full_version. Please invoke again with DD_AGENT_MAJOR_VERSION=7${NC}\n"
        exit 1
    else
        if [ -z "${agent_minor_version}" ]; then
            dmg_version="7-latest"
        fi
    fi
fi

if [ -z "$dmg_version" ]; then
    if [ -z "$agent_patch_version" ]; then
        agent_patch_version=$(find_latest_patch_version_for "${agent_major_version}.${agent_minor_version_without_patch}")
        if [ -z "$agent_patch_version" ] || [ "$agent_patch_version" -lt 0 ]; then
            echo -e "${YELLOW}Warning: Failed to obtain latest patch version for Agent ${agent_major_version}.${agent_minor_version_without_patch}. Defaulting to '0'.${NC}"
            agent_patch_version=0
        fi
    fi
    # Check if the version is a classic release version or a pre-release version
    if [ "$agent_minor_version" = "$clean_agent_minor_version" ];then
        dmg_version="${agent_major_version}.${agent_minor_version_without_patch}.${agent_patch_version}-1"
    else
        dmg_version="${agent_major_version}.${agent_minor_version}-1"
    fi
fi

# COMPAT: agent 5 used DD_UPGRADE + datadog.conf. Can be removed once agent 5 upgrades are no longer supported.
if [ "$upgrade" ]; then
    if [ ! -f $etc_dir/datadog.conf ]; then
        printf "${RED}DD_UPGRADE set but no config was found at $etc_dir/datadog.conf.${NC}\n"
        exit 1;
    fi
fi

if [ ! "$apikey" ]; then
    # if it's an upgrade, then we will use the transition script
    if [ ! "$upgrade" ]; then
        printf "${RED}API key not available in DD_API_KEY environment variable.${NC}\n"
        exit 1;
    fi
fi



# SUDO_USER is defined in man sudo: https://linux.die.net/man/8/sudo
# "SUDO_USER Set to the login name of the user who invoked sudo."

# USER is defined in man login: https://ss64.com/osx/login.html
# "Login enters information into the environment (see environ(7))
#  specifying the user's home directory (HOME), command interpreter (SHELL),
#  search path (PATH), terminal type (TERM) and user name (both LOGNAME and USER)."

# We want to get the real user who executed the command. Two situations can happen:
# - the command was run as the current user: then $USER contains the user which launched the command, and $SUDO_USER is empty,
# - the command was run with sudo: then $USER contains the name of the user targeted by the sudo command (by default, root)
#   and $SUDO_USER contains the user which launched the sudo command.
# The following block covers both cases so that we have tbe username we want in the real_user variable.
real_user=`if [ "$SUDO_USER" ]; then
  echo "$SUDO_USER"
else
  echo "$USER"
fi`
cmd_real_user="sudo -Eu $real_user"

TMPDIR=`sudo -u "$real_user" getconf DARWIN_USER_TEMP_DIR`
export TMPDIR

# In order to install with the right user
rm -f /tmp/datadog-install-user
echo "$real_user" > /tmp/datadog-install-user

function on_error() {
    printf "${RED}$ERROR_MESSAGE
It looks like you hit an issue when trying to install the Agent.

Troubleshooting and basic usage information for the Agent are available at:

    https://docs.datadoghq.com/agent/basic_agent_usage/

If you're still having problems, please send an email to support@datadoghq.com
with the contents of ddagent-install.log and we'll do our very best to help you
solve your problem.\n${NC}\n"
}
trap on_error ERR

cmd_agent="$cmd_real_user /opt/datadog-agent/bin/agent/agent"

function sed_inplace_arg() {
    # Check for vanilla OS X sed or GNU sed
    if [ "$(sed --version 2>/dev/null | grep -c "GNU")" -ne 0 ]; then
        echo "-i"
    fi

    echo "-i ''"
}

function new_config() {
    i_cmd="$(sed_inplace_arg)"
    $sudo_cmd sh -c "sed $i_cmd 's/api_key:.*/api_key: $apikey/' \"$etc_dir/datadog.yaml\""
    if [ "$site" ]; then
        $sudo_cmd sh -c "sed $i_cmd 's/# site:.*/site: $site/' \"$etc_dir/datadog.yaml\""
    fi
    $sudo_cmd chown "$real_user":admin "$etc_dir/datadog.yaml"
    $sudo_cmd chmod 660 $etc_dir/datadog.yaml
}

# COMPAT: agent 5 used datadog.conf; this converts it to datadog.yaml. Can be removed once agent 5 upgrades are no longer supported.
function import_config() {
    printf "${BLUE}\n* Converting old datadog.conf file to new datadog.yaml format\n${NC}\n"
    $cmd_agent import $etc_dir $etc_dir -f
}

# Determine agent flavor to install
if [ -z "$agent_dist_channel" ]; then
    dmg_url_prefix="$dmg_base_url/datadog-agent-${dmg_version}"
else
    dmg_url_prefix="$dmg_base_url/$agent_dist_channel/datadog-agent-${dmg_version}"
fi

dmg_url="$dmg_url_prefix.$arch.dmg"  # favor architecture-specific DMG, if available
if [ "$(curl --head --location --output /dev/null "${curl_retries[@]}" --silent --write-out '%{http_code}' "$dmg_url")" != 200 ]; then
    dmg_url="$dmg_url_prefix.dmg"  # fallback to "universal" DMG
    if [ "$arch" = arm64 ] && ! /usr/bin/pgrep oahd >/dev/null 2>&1; then
        printf "${RED}Rosetta is needed to run datadog-agent on $arch.\nYou can install it by running the following command :\n/usr/sbin/softwareupdate --install-rosetta --agree-to-license${NC}\n"
        exit 1
    fi
fi

# # Install the agent
printf "${BLUE}\n* Downloading datadog-agent\n${NC}"
prepare_dmg_file $dmg_file
if ! $sudo_cmd curl --fail --progress-bar "$dmg_url" "${curl_retries[@]}" --output $dmg_file; then
    printf "${RED}Couldn't download the installer for macOS Agent version ${dmg_version}.${NC}\n"
    exit 1;
fi
printf "${BLUE}\n* Installing datadog-agent, you might be asked for your sudo password...\n${NC}"
$sudo_cmd hdiutil detach "/Volumes/datadog_agent" >/dev/null 2>&1 || true
printf "${BLUE}\n    - Mounting the DMG installer...\n${NC}"
$sudo_cmd hdiutil attach "$dmg_file" -mountpoint "/Volumes/datadog_agent" >/dev/null
printf "${BLUE}\n    - Unpacking and copying files (this usually takes about a minute) ...\n${NC}"
# COMPAT: old DMG postinst checks for /tmp/install-ddagent/system-wide to take the system-wide path.
# Can be removed once all supported DMG versions have the updated postinst baked in.
$sudo_cmd mkdir -p /tmp/install-ddagent
$sudo_cmd touch /tmp/install-ddagent/system-wide
cd / && $sudo_cmd /usr/sbin/installer -pkg "`find "/Volumes/datadog_agent" -name \*.pkg 2>/dev/null`" -target / >/dev/null
$sudo_cmd rm -rf /tmp/install-ddagent
printf "${BLUE}\n    - Unmounting the DMG installer ...\n${NC}"
$sudo_cmd hdiutil detach "/Volumes/datadog_agent" >/dev/null

# Creating or overriding the install information
install_info_content="---
install_method:
  tool: install_script_mac
  tool_version: install_script_mac
  installer_version: install_script_mac-$install_script_version
"
$sudo_cmd sh -c "echo '$install_info_content' > $etc_dir/install_info"
$sudo_cmd chown "$real_user":admin "$etc_dir/install_info"
$sudo_cmd chmod 660 $etc_dir/install_info

# Set the configuration
if grep -E 'api_key:( APIKEY)?$' "$etc_dir/datadog.yaml" > /dev/null 2>&1; then
    if [ "$upgrade" ]; then
        import_config
    else
        new_config
    fi
    printf "\n${BLUE}* Agent will be started after service setup ...\n${NC}\n"
else
    printf "${BLUE}\n* A datadog.yaml configuration file already exists. It will not be overwritten.\n${NC}\n"
fi

# Restart agent to pick up the injected API key (agent was started by postinst)
printf "${BLUE}\n* Restarting agent to apply configuration...\n${NC}"
$sudo_cmd launchctl kickstart -k "system/$service_name"


# Set up and start the system-probe service if this version includes support for it
sysprobe_plist_example_file="${etc_dir}/${sysprobe_service_name}.plist.example"
if [ -f "$sysprobe_plist_example_file" ]; then
    printf "${BLUE}\n* Setting up system-probe ($sysprobe_service_name) as a system-wide LaunchDaemon ...\n\n${NC}"
    $sudo_cmd mv "$sysprobe_plist_example_file" "$sysprobe_servicefile_name"
    $sudo_cmd chown "0:0" "$sysprobe_servicefile_name"
    $sudo_cmd chmod 644 "$sysprobe_servicefile_name"
    $sudo_cmd launchctl load -w "$sysprobe_servicefile_name"
    $sudo_cmd launchctl kickstart "system/$sysprobe_service_name"
fi

# Agent works, echo some instructions and exit
printf "${GREEN}

Your Agent is running properly. It will continue to run in the
background and submit metrics to Datadog.

You can check the agent status using the \"datadog-agent status\" command
or by opening the webui using the \"datadog-agent launch-gui\" command.

${NC}"

printf "${GREEN}
If you ever want to stop the Agent, please use the launchctl command.
The Agent will start automatically at system startup.
${NC}"
