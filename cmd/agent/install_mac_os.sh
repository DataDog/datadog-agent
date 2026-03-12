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

DDAGENT_USER="_datadog"
DDAGENT_GROUP="daemon"  # GID 1, macOS built-in system daemon group

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

# shellcheck disable=SC2016
install_user_home=$($cmd_real_user bash -c 'echo "$HOME"')
# shellcheck disable=SC2016
user_uid=$($cmd_real_user bash -c 'id -u')
user_plist_file=${install_user_home}/Library/LaunchAgents/${service_name}.plist

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

cmd_launchctl="$cmd_real_user launchctl"

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

# COMPAT: used to inject UserName/GroupName into DMG plist templates that predate baking them in at build time.
# Only called when the keys are absent (see call site). Can be removed once all supported DMG versions
# have UserName/GroupName baked into launchd.plist.example.in.
function plist_modify_user_group() {
    plist_file="$1"
    user_value="$2"
    group_value="$3"
    user_parameter="UserName"
    group_parameter="GroupName"

    terms="UserName GroupName"
    for term in $terms; do
        if grep "<key>$term</key>" "$1"; then
            printf "${RED}$plist_file already contains <key>$term</key>; it should not be called for plists that already have these keys${NC}\n"
            return 1
        fi
    done

    ## to insert user/group into the xml file, we'll find the last "</dict>" occurrence and insert before it
    i_cmd="$(sed_inplace_arg)"
    closing_dict_line=$($sudo_cmd cat "$plist_file" | grep -n "</dict>" | tail -1 | cut -f1 -d:)
    $sudo_cmd sh -c "sed $i_cmd -e \"${closing_dict_line},${closing_dict_line}s|</dict>|<key>$user_parameter</key><string>$user_value</string>\n</dict>|\" -e \"${closing_dict_line},${closing_dict_line}s|</dict>|<key>$group_parameter</key><string>$group_value</string>\n</dict>|\" \"$plist_file\""
}

function create_ddagent_user() {
    if id -u "$DDAGENT_USER" >/dev/null 2>&1; then
        printf "${BLUE}* User $DDAGENT_USER already exists, skipping creation\n${NC}"
        return 0
    fi

    printf "${BLUE}* Creating $DDAGENT_USER system user...\n${NC}"

    # Find a free UID in the system range (300-499)
    local uid=300
    while dscl . -list /Users UniqueID | awk '{print $2}' | grep -qx "$uid"; do
        uid=$((uid + 1))
    done
    if [ "$uid" -ge 500 ]; then
        printf "${RED}No free UID available in range 300-499 for $DDAGENT_USER${NC}\n"
        exit 1
    fi

    $sudo_cmd dscl . -create /Users/$DDAGENT_USER
    $sudo_cmd dscl . -create /Users/$DDAGENT_USER UserShell /usr/bin/false
    $sudo_cmd dscl . -create /Users/$DDAGENT_USER RealName "Datadog Agent"
    $sudo_cmd dscl . -create /Users/$DDAGENT_USER UniqueID "$uid"
    $sudo_cmd dscl . -create /Users/$DDAGENT_USER PrimaryGroupID 1
    $sudo_cmd dscl . -create /Users/$DDAGENT_USER NFSHomeDirectory /var/empty
    $sudo_cmd dscl . -create /Users/$DDAGENT_USER IsHidden 1

    printf "${GREEN}  ✓ Created $DDAGENT_USER (UID $uid)\n${NC}"
}

# Function: cleanup_per_user_installation
# Description: Removes per-user Datadog Agent and GUI LaunchAgent plists
# Arguments:
#   $1 - username (the user whose installation to clean up)
#   $2 - user_home (the user's home directory path)
#   $3 - user_uid (the user's UID)
# Returns: 0 on success
function cleanup_per_user_installation() {
    local username="$1"
    local user_home="$2"
    local uid="$3"
    local cleaned=false

    # Check and remove per-user agent plist
    local agent_plist="$user_home/Library/LaunchAgents/com.datadoghq.agent.plist"
    if [ -f "$agent_plist" ]; then
        printf "${YELLOW}      - Removing per-user agent for user '%s'...\n${NC}" "$username"
        # Try to bootout if user is logged in (graceful stop)
        $sudo_cmd launchctl bootout "gui/$uid/com.datadoghq.agent" 2>/dev/null || true
        rm -f "$agent_plist"
        cleaned=true
    fi

    # Check and remove per-user GUI plist
    local gui_plist="$user_home/Library/LaunchAgents/com.datadoghq.gui.plist"
    if [ -f "$gui_plist" ]; then
        printf "${YELLOW}      - Removing per-user GUI for user '%s'...\n${NC}" "$username"
        # Try to bootout if user is logged in (graceful stop)
        $sudo_cmd launchctl bootout "gui/$uid/com.datadoghq.gui" 2>/dev/null || true
        rm -f "$gui_plist"
        cleaned=true
    fi

    if [ "$cleaned" = true ]; then
        printf "${GREEN}      ✓ Cleaned up per-user installation for user '%s'\n${NC}" "$username"
    fi

    return 0
}

# Function: cleanup_all_per_user_installations
# Description: Iterates through all user home directories and removes per-user installations
# This prevents conflicts when system-wide installation is performed
# Arguments: None
# Returns: 0 on success
function cleanup_all_per_user_installations() {
    printf "${BLUE}    - Cleaning up per-user installations for all users...\n${NC}"

    local found_installations=false
    local current_install_uid="$user_uid"

    # Iterate through all user home directories
    for user_home in /Users/*; do
        # Skip if not a directory or doesn't have Library folder
        if [ ! -d "$user_home" ] || [ ! -d "$user_home/Library" ]; then
            continue
        fi

        local username
        username=$(basename "$user_home")

        # Get user UID
        local user_uid_check
        user_uid_check=$(id -u "$username" 2>/dev/null)

        # Skip if:
        # - User doesn't exist
        # - UID < 500 (system accounts)
        # - Is the currently installing user (their plist will be moved to system location)
        if [ -z "$user_uid_check" ] || [ "$user_uid_check" -lt 500 ] || [ "$user_uid_check" = "$current_install_uid" ]; then
            continue
        fi

        # Check if this user has per-user installations
        if [ -f "$user_home/Library/LaunchAgents/com.datadoghq.agent.plist" ] || \
           [ -f "$user_home/Library/LaunchAgents/com.datadoghq.gui.plist" ]; then
            found_installations=true
            cleanup_per_user_installation "$username" "$user_home" "$user_uid_check"
        fi
    done

    if [ "$found_installations" = false ]; then
        printf "${BLUE}    - No per-user installations found in other user accounts\n${NC}"
    else
        printf "${GREEN}    ✓ All per-user installations cleaned up\n${NC}"
    fi

    return 0
}

# Function: cleanup_stale_sockets
# Description: Removes stale GUI socket files from the IPC directory
# This prevents "Address already in use" errors during reinstallation
# Should be called after GUI processes are confirmed stopped
# Arguments: None
# Returns: 0 on success
function cleanup_stale_sockets() {
    local ipc_dir="$run_dir/ipc"

    if [ ! -d "$ipc_dir" ]; then
        # IPC directory doesn't exist, nothing to clean
        return 0
    fi

    # Check if any socket files exist
    local socket_files=("$ipc_dir"/gui-*.sock)
    if [ -e "${socket_files[0]}" ]; then
        printf "${BLUE}    - Cleaning up stale socket files...\n${NC}"
        $sudo_cmd rm -f "$ipc_dir"/gui-*.sock
        printf "${GREEN}      ✓ Stale sockets removed\n${NC}"
    fi

    return 0
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

# Create the _datadog system user if it doesn't already exist
create_ddagent_user

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
if [ -f "$systemwide_servicefile_name" ]; then
    printf "${BLUE}\n    - Stopping system-wide Datadog Agent daemon ...\n${NC}"
    # we use "|| true" because if the service is not started/loaded, the commands fail
    $sudo_cmd launchctl stop $service_name || true
    if $sudo_cmd launchctl print system/$service_name 2>/dev/null >/dev/null; then
        $sudo_cmd launchctl unload -wF $systemwide_servicefile_name || true
    fi
fi

# Shut down and remove system probe service if present
if [ -f "$sysprobe_servicefile_name" ]; then
    printf "${BLUE}\n    - Stopping System Probe daemon ...\n${NC}"
    $sudo_cmd launchctl stop $sysprobe_service_name || true
    if $sudo_cmd launchctl print system/$sysprobe_service_name 2>/dev/null >/dev/null; then
        $sudo_cmd launchctl unload -wF $sysprobe_servicefile_name || true
    fi
    $sudo_cmd rm -f "${sysprobe_servicefile_name}"
fi

# Stop GUI app before installation/upgrade.
# COMPAT: $user_plist_file check handles upgrades from per-user installations (pre-_datadog user).
if [ -f "$user_plist_file" ] || [ -f "/Library/LaunchAgents/com.datadoghq.gui.plist" ]; then
    printf "${BLUE}\n    - Stopping GUI app ...\n${NC}"

    # Method 1: Try current user (most common case)
    $cmd_launchctl bootout "gui/$user_uid/com.datadoghq.gui" 2>/dev/null || true

    # Method 2: Try all logged-in users (multi-user system-wide installations)
    for logged_user in $(who | awk '{print $1}' | sort -u); do
        logged_uid=$(id -u "$logged_user" 2>/dev/null)
        if [ -n "$logged_uid" ] && [ "$logged_uid" != "$user_uid" ]; then
            $sudo_cmd launchctl bootout "gui/$logged_uid/com.datadoghq.gui" 2>/dev/null || true
        fi
    done

    # Wait for GUI processes to actually terminate (with 10 second timeout to match ExitTimeOut)
    max_wait=100  # 100 * 0.1s = 10 seconds
    count=0
    while [ $count -lt $max_wait ]; do
        if ! pgrep -f "Datadog Agent.app/Contents/MacOS/gui" > /dev/null 2>&1; then
            break
        fi
        sleep 0.1
        count=$((count + 1))
    done

    if [ $count -ge $max_wait ]; then
        printf "${YELLOW}    Warning: GUI processes still running after 10s, proceeding anyway${NC}\n"
    fi

    # Clean up stale socket files to prevent binding conflicts
    cleanup_stale_sockets
fi

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

# Install $service_name as a system-wide LaunchDaemon
printf "${BLUE}\n* Installing $service_name as a system-wide LaunchDaemon ...\n\n${NC}"
# COMPAT: agents < 7.52.0 ran as a per-user LaunchAgent; remove the login item and unload if still present.
if $cmd_launchctl print "gui/$user_uid/$service_name" 1>/dev/null 2>/dev/null; then
    $cmd_real_user osascript -e 'tell application "System Events" to if login item "Datadog Agent" exists then delete login item "Datadog Agent"'
    $cmd_launchctl stop "$service_name"
    $cmd_launchctl unload "$user_plist_file"
fi

# COMPAT: clean up the installing user's per-user GUI plist from pre-_datadog installations.
installing_user_gui_plist="${install_user_home}/Library/LaunchAgents/com.datadoghq.gui.plist"
if [ -f "$installing_user_gui_plist" ]; then
    printf "${BLUE}    - Removing installing user's per-user GUI plist...\n${NC}"
    rm -f "$installing_user_gui_plist"
    printf "${GREEN}      ✓ Per-user GUI plist removed\n${NC}"
fi

# COMPAT: remove per-user agent plists for all other users from pre-_datadog installations.
cleanup_all_per_user_installations

# Move the plist file to the system location
$sudo_cmd mv "$user_plist_file" /Library/LaunchDaemons/
# COMPAT: inject UserName/GroupName if the DMG plist template predates having them baked in.
# Can be removed once all supported DMG versions include UserName/GroupName in launchd.plist.example.in.
if ! $sudo_cmd grep -q "<key>UserName</key>" "$systemwide_servicefile_name"; then
    plist_modify_user_group "$systemwide_servicefile_name" "$DDAGENT_USER" "$DDAGENT_GROUP"
fi
$sudo_cmd chown "0:0" "$systemwide_servicefile_name"
$sudo_cmd chmod 644 "$systemwide_servicefile_name"
$sudo_cmd chown -R "$DDAGENT_USER:admin" "$etc_dir" "$log_dir" "$run_dir"
$sudo_cmd chmod 770 "$etc_dir" "$log_dir"
$sudo_cmd chmod 775 "$run_dir"
$sudo_cmd find "$etc_dir" -type f -exec $sudo_cmd chmod 660 {} \;
$sudo_cmd launchctl load -w "$systemwide_servicefile_name"
$sudo_cmd launchctl kickstart "system/$service_name"

# Try to load headless GUI app for current user if they have a user session
# The headless GUI LaunchAgent was installed in /Library/LaunchAgents/ by postinst
if $cmd_real_user launchctl managername | grep -q "Aqua"; then
    printf "${BLUE}\n* User session detected, loading headless GUI app for current user...\n${NC}"
    $cmd_real_user launchctl load -w /Library/LaunchAgents/com.datadoghq.gui.plist
else
    printf "${BLUE}\n* No user session detected, headless GUI app will launch at next user login\n${NC}"
fi

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
