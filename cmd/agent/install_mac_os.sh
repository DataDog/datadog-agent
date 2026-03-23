#!/usr/bin/env bash
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# Datadog Agent install script for macOS.
set -e
install_script_version=2.0.0

# Terminal color detection
# Colors are enabled only when outputting to a terminal (not when piped/redirected)
if [ -t 1 ]; then
    RED='\033[31m'
    GREEN='\033[32m'
    YELLOW='\033[33m'
    BLUE='\033[34m'
    NC='\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi
install_log_file=/tmp/ddagent-install.log
exec > >(tee "$install_log_file") 2>&1
dmg_file=/tmp/datadog-agent.dmg
dmg_base_url="https://s3.amazonaws.com/dd-agent"
# Root-only staging directory for install-time data (API key, saved config).
# /private/var/root is mode 700 owned by root:wheel, preventing local attackers
# from reading secrets or planting symlinks in /tmp.
install_staging_dir="/private/var/root/datadog-install"
install_env_file="$install_staging_dir/env"

if [ -n "$DD_REPO_URL" ]; then
    dmg_base_url=$DD_REPO_URL
fi

# Optional: path to a local DMG file to use instead of downloading
local_dmg_path=
if [ -n "$DD_DMG_PATH" ]; then
    local_dmg_path="$DD_DMG_PATH"
    if [ ! -f "$local_dmg_path" ]; then
        printf "${RED}DD_DMG_PATH is set but file does not exist: %s${NC}\\n" "$local_dmg_path"
        exit 1
    fi
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

gui_app_menu_enabled=false
if [ "$DD_GUI_APP_MENU_ENABLED" = "true" ]; then
    gui_app_menu_enabled=true
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

# Version guard: this script is for Agent 7.79.0+
if [ -n "$DD_AGENT_MINOR_VERSION" ] && [ "$agent_minor_version_without_patch" -lt 79 ]; then
    printf "${RED}This install script is for Agent 7.79.0 and later.
For older versions, use install_mac_os_old.sh instead.
If you are downgrading from Agent >= 7.79.0, you must fully uninstall the Agent first:
    https://docs.datadoghq.com/agent/supported_platforms/osx/#uninstall-the-agent${NC}\n"
    exit 1
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

if [ "${macos_major_version}" -lt 12 ]; then
    echo -e "${RED}Datadog Agent requires macOS 12.0 or later.${NC}\n"
    exit 1
fi

# Determine version to download (always Agent 7)
dmg_version=
if [ -n "$DD_AGENT_MINOR_VERSION" ]; then
    if [ -z "$agent_patch_version" ]; then
        agent_patch_version=$(find_latest_patch_version_for "7.${agent_minor_version_without_patch}")
        if [ -z "$agent_patch_version" ] || [ "$agent_patch_version" -lt 0 ]; then
            echo -e "${YELLOW}Warning: Failed to obtain latest patch version for Agent 7.${agent_minor_version_without_patch}. Defaulting to '0'.${NC}"
            agent_patch_version=0
        fi
    fi
    if [ "$agent_minor_version" = "$clean_agent_minor_version" ]; then
        dmg_version="7.${agent_minor_version_without_patch}.${agent_patch_version}-1"
    else
        dmg_version="7.${agent_minor_version}-1"
    fi
else
    dmg_version="7-latest"
fi

if [ -z "$apikey" ]; then
    printf "${RED}API key not available in DD_API_KEY environment variable.${NC}\n"
    exit 1
fi

function on_error() {
    printf "${RED}
It looks like you hit an issue when trying to install the Agent.
See the following log files for details:

    - $install_log_file
    - /opt/datadog-agent/logs/preinstall.log
    - /opt/datadog-agent/logs/postinstall.log

If you're still having problems, please send an email to support@datadoghq.com
with the contents of the log files and we'll do our very best to help
you solve your problem.
${NC}\n"
}
trap on_error ERR

# Clean up sensitive staging files on any exit (success, error, or signal).
# The staging dir contains the API key and must not be left on disk.
# The postinst script cleans it after reading, but if the install fails before
# that point, this trap ensures cleanup still happens.
function cleanup() {
    $sudo_cmd rm -rf "$install_staging_dir"
    $sudo_cmd rm -f "$dmg_file"
}
trap cleanup EXIT

# Determine agent flavor to install (skipped when using a local DMG)
if [ -z "$local_dmg_path" ]; then
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
fi

# Write configuration for the pkg's postinst to consume
$sudo_cmd rm -rf "$install_staging_dir"
$sudo_cmd mkdir -p "$install_staging_dir"
$sudo_cmd chmod 700 "$install_staging_dir"
{
    echo "DD_API_KEY=$apikey"
    [ -n "$site" ] && echo "DD_SITE=$site"
    [ "$gui_app_menu_enabled" = true ] && echo "DD_GUI_APP_MENU_ENABLED=true"
    echo "DD_INSTALL_METHOD=install_script_mac"
    echo "DD_INSTALL_SCRIPT_VERSION=$install_script_version"
} | $sudo_cmd tee "$install_env_file" > /dev/null
$sudo_cmd chmod 600 "$install_env_file"

# Download and install
if [ -n "$local_dmg_path" ]; then
    printf "${BLUE}\n* Using local DMG: %s\n${NC}" "$local_dmg_path"
    dmg_file="$local_dmg_path"
else
    printf "${BLUE}\n* Downloading datadog-agent ${dmg_version}\n${NC}"
    prepare_dmg_file $dmg_file
    if ! $sudo_cmd curl --fail --progress-bar "$dmg_url" "${curl_retries[@]}" --output $dmg_file; then
        printf "${RED}Couldn't download the installer for macOS Agent version ${dmg_version}.${NC}\n"
        exit 1;
    fi
fi
printf "${BLUE}\n* Installing datadog-agent, you might be asked for your sudo password...\n${NC}"
$sudo_cmd hdiutil detach "/Volumes/datadog_agent" >/dev/null 2>&1 || true
printf "${BLUE}\n    - Mounting the DMG installer...\n${NC}"
$sudo_cmd hdiutil attach "$dmg_file" -mountpoint "/Volumes/datadog_agent" -nobrowse >/dev/null
printf "${BLUE}\n    - Unpacking and copying files (this usually takes about a minute) ...\n${NC}"
cd / && $sudo_cmd /usr/sbin/installer -pkg "`find "/Volumes/datadog_agent" -name \*.pkg 2>/dev/null`" -target / >/dev/null
printf "${BLUE}\n    - Unmounting the DMG installer ...\n${NC}"
$sudo_cmd hdiutil detach "/Volumes/datadog_agent" >/dev/null

if $sudo_cmd launchctl print system/com.datadoghq.agent 2>/dev/null | grep -q "pid ="; then
    printf "${GREEN}

Your Agent is running properly. It will continue to run in the
background and submit metrics to Datadog.

You can check the agent status using the \"datadog-agent status\" command
or by opening the webui using the \"datadog-agent launch-gui\" command.

To stop the Agent:  sudo launchctl kill SIGTERM system/com.datadoghq.agent
To start the Agent: sudo launchctl kickstart system/com.datadoghq.agent
The Agent will start automatically at system startup.

Troubleshooting information for the Agent is available at:
    https://docs.datadoghq.com/agent/troubleshooting/
${NC}"
else
    printf "${YELLOW}

WARNING: The Agent was installed successfully, but the agent service
failed to start.

Check the postinstall log for details:
    /opt/datadog-agent/logs/postinstall.log

Troubleshooting information for the Agent is available at:
    https://docs.datadoghq.com/agent/troubleshooting/
${NC}"
fi
