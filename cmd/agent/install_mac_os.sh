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

request_location_permission_value=
if [ -n "$DD_REQUEST_LOCATION_PERMISSION" ]; then
    case "$(echo "$DD_REQUEST_LOCATION_PERMISSION" | tr '[:upper:]' '[:lower:]')" in
        false|0|no) request_location_permission_value=false ;;
        *) request_location_permission_value=true ;;
    esac
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

# Cleanup tmp files used for installation
rm -f /tmp/install-ddagent/system-wide

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

systemdaemon_install=false
systemdaemon_user_group=
if [ -n "$DD_SYSTEMDAEMON_INSTALL" ]; then
    systemdaemon_install=$DD_SYSTEMDAEMON_INSTALL
    if [ -n "$DD_SYSTEMDAEMON_USER_GROUP" ]; then
        systemdaemon_user_group=$DD_SYSTEMDAEMON_USER_GROUP
    else
        printf "${RED}DD_SYSTEMDAEMON_INSTALL set without DD_SYSTEDAEMON_USER_GROUP${NC}\n"
        exit 1;
    fi
    if ! echo "$systemdaemon_user_group" | grep "^[^:]\+:[^:]\+$" > /dev/null; then
        printf "${RED}DD_SYSTEMDAEMON_USER_GROUP must be in format UserName:GroupName${NC}\n"
        exit 1;
    fi
    if echo "$systemdaemon_user_group" | grep ">\|<" > /dev/null; then
        printf "${RED}DD_SYSTEMDAEMON_USER_GROUP can't contain '>' or '<', because it will be used in XML file${NC}\n"
        exit 1;
    fi
    systemdaemon_user="$(echo "$systemdaemon_user_group" | awk -F: '{ print $1 }')"
    systemdaemon_group="$(echo "$systemdaemon_user_group" | awk -F: '{ print $2 }')"
    if ! id -u "$systemdaemon_user" >/dev/null 2>&1 ; then
        printf "${RED}User $systemdaemon_user not found, can't proceed with installation${NC}\n"
        exit 1;
    fi
    # dscacheutil -q group output is in form:
    #   name: groupname
    #   password: *
    #   gid: 1001
    #   users: user1 user2
    # so we use `grep` and `awk` to get the group name
    if ! dscacheutil -q group | grep "name:" | awk '{print $2}' | grep -w "$systemdaemon_group" >/dev/null 2>&1; then
        printf "${RED}Group $systemdaemon_group not found, can't proceed with installation${NC}\n"
        exit 1;
    fi
fi

if [ "$systemdaemon_install" != false ]; then
  mkdir -p /tmp/install-ddagent
  touch /tmp/install-ddagent/system-wide
fi

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

# Deprecation warning for per-user installations
if [ "$systemdaemon_install" = false ] && [ ! "$upgrade" ]; then
    printf "${YELLOW}
================================================================================
WARNING: Per-User Installation Mode
================================================================================
Per-user installations may be deprecated in the future.
We recommend using system-wide installation instead going forward.

    DD_SYSTEMDAEMON_INSTALL=true DD_SYSTEMDAEMON_USER_GROUP=<user>:staff \\
        bash install_mac_os.sh

System-wide installation provides:
  - Better multi-user support
  - Consistent behavior across user sessions
  - Improved security and resource management

Continuing with per-user installation in 10 seconds...
Press Ctrl+C to cancel and switch to system-wide installation.
================================================================================
${NC}\n"
    sleep 10
fi

if [ "$systemdaemon_install" == false ] && [ -f "$systemwide_servicefile_name" ]; then
    printf "${RED}
$systemwide_servicefile_name exists, suggesting a
system-wide Agent installation is present. Individual users
can't install the Agent when system-wide installation exists.

To proceed, uninstall the system-wide agent first:
    sudo launchctl unload -w /Library/LaunchDaemons/com.datadoghq.agent.plist
    sudo rm /Library/LaunchDaemons/com.datadoghq.agent.plist
    sudo rm -f /Library/LaunchAgents/com.datadoghq.gui.plist

Then rerun this script.
${NC}\n"

    exit 1;
fi

# Check for system-wide GUI installation (from partial/failed installations)
if [ "$systemdaemon_install" == false ] && [ -f "/Library/LaunchAgents/com.datadoghq.gui.plist" ]; then
    printf "${RED}
System-wide GUI installation detected at:
    /Library/LaunchAgents/com.datadoghq.gui.plist

This may be from a partial or incomplete system-wide installation.
Cannot proceed with per-user installation.

To proceed, remove the system-wide components:
    sudo launchctl bootout system/com.datadoghq.agent 2>/dev/null || true
    sudo rm -f /Library/LaunchDaemons/com.datadoghq.agent.plist
    sudo rm -f /Library/LaunchAgents/com.datadoghq.gui.plist

Then rerun this script.
${NC}\n"

    exit 1;
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
# If this is a systemwide install done over SSH or a similar method, the real_user is now root
# which will eventually make the installation fail in the postinstall script. In this case, we
# set real_user to the target user of the systemwide installation.
if [ "$systemdaemon_install" = true ] && [ "$real_user" = root ]; then
    real_user="$(echo "$systemdaemon_user_group" | awk -F: '{ print $1 }')"
    # The install will copy plist file to real_user home dir => we add `-H`
    # as a sudo argument to properly get its home for access to the plist file.
    cmd_real_user="sudo -EHu $real_user"
else
    cmd_real_user="sudo -Eu $real_user"
fi

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
    if [ -n "$request_location_permission_value" ]; then
        $sudo_cmd sh -c "sed $i_cmd -E 's/^#?[[:space:]]*request_location_permission:[[:space:]]*.*/request_location_permission: $request_location_permission_value/' \"$etc_dir/datadog.yaml\""
    fi
    $sudo_cmd chown "$real_user":admin "$etc_dir/datadog.yaml"
    $sudo_cmd chmod 640 $etc_dir/datadog.yaml
}

function import_config() {
    printf "${BLUE}\n* Converting old datadog.conf file to new datadog.yaml format\n${NC}\n"
    $cmd_agent import $etc_dir $etc_dir -f
}

function plist_modify_user_group() {
    plist_file="$1"
    user_value="$2"
    group_value="$3"
    user_parameter="UserName"
    group_parameter="GroupName"

    # if, in a future agent version we add UserName/GroupName to the plist file,
    # we want this older version of install script fail, because it wouldn't know what to do
    terms="UserName GroupName"
    for term in $terms; do
        if grep "<key>$term</key>" "$1"; then
            printf "${RED}$plist_file already contains <key>$term</key>, please update this script to the latest version${NC}\n"
            return 1
        fi
    done

    ## to insert user/group into the xml file, we'll find the last "</dict>" occurrence and insert before it
    i_cmd="$(sed_inplace_arg)"
    closing_dict_line=$($sudo_cmd cat "$plist_file" | grep -n "</dict>" | tail -1 | cut -f1 -d:)
    $sudo_cmd sh -c "sed $i_cmd -e \"${closing_dict_line},${closing_dict_line}s|</dict>|<key>$user_parameter</key><string>$user_value</string>\n</dict>|\" -e \"${closing_dict_line},${closing_dict_line}s|</dict>|<key>$group_parameter</key><string>$group_value</string>\n</dict>|\" \"$plist_file\""
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
if [ "$systemdaemon_install" != false ] && [ -f "$systemwide_servicefile_name" ]; then
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

# Stop GUI app before installation/upgrade (handles both per-user and system-wide modes)
# In system-wide mode, GUI runs for each logged-in user; we need to stop all instances
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
cd / && $sudo_cmd /usr/sbin/installer -pkg "`find "/Volumes/datadog_agent" -name \*.pkg 2>/dev/null`" -target / >/dev/null
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
$sudo_cmd chmod 640 $etc_dir/install_info

# Set the configuration
if grep -E 'api_key:( APIKEY)?$' "$etc_dir/datadog.yaml" > /dev/null 2>&1; then
    if [ "$upgrade" ]; then
        import_config
    else
        new_config
    fi
    printf "\n${BLUE}* Restarting the Agent...\n${NC}\n"
    # systemwide installation is stopped at this point and will be started later on
    if [ "$systemdaemon_install" != true ]; then
      $cmd_launchctl stop $service_name

      # Wait for the agent to fully stop
      retry=0
      until [ "$retry" -ge 5 ]; do
          curl -m 5 -o /dev/null -s -I http://127.0.0.1:5002 || break
          retry=$[$retry+1]
          sleep 5
      done
      if [ "$retry" -ge 5 ]; then
          printf "\n${YELLOW}Could not restart the agent.
You may have to restart it manually using the systray app or the
\"launchctl start $service_name\" command.\n${NC}\n"
      fi

      $cmd_launchctl start $service_name
    fi
else
    printf "${BLUE}\n* A datadog.yaml configuration file already exists. It will not be overwritten.\n${NC}\n"
fi

# Starting the app
if [ "$systemdaemon_install" = false ]; then
    $cmd_real_user open -a 'Datadog Agent.app'
else
    printf "${BLUE}\n* Installing $service_name as a system-wide LaunchDaemon ...\n\n${NC}"
    # Remove the Agent login item and unload the agent for current user
    # if it is running - it's not running if the script was launched when
    # the GUI was not running for the user (e.g. a run of this script via
    # ssh for user not logged in via GUI).
    # This condition is true only when installing an agent < 7.52.0
    if $cmd_launchctl print "gui/$user_uid/$service_name" 1>/dev/null 2>/dev/null; then
        $cmd_real_user osascript -e 'tell application "System Events" to if login item "Datadog Agent" exists then delete login item "Datadog Agent"'
        $cmd_launchctl stop "$service_name"
        $cmd_launchctl unload "$user_plist_file"
    fi

    # Clean up installing user's per-user GUI plist (if exists from previous per-user installation)
    installing_user_gui_plist="${install_user_home}/Library/LaunchAgents/com.datadoghq.gui.plist"
    if [ -f "$installing_user_gui_plist" ]; then
        printf "${BLUE}    - Removing installing user's per-user GUI plist...\n${NC}"
        rm -f "$installing_user_gui_plist"
        printf "${GREEN}      ✓ Per-user GUI plist removed\n${NC}"
    fi

    # Clean up per-user installations for ALL users on the system
    # This prevents conflicts when other users log in after system-wide installation
    cleanup_all_per_user_installations

    # move the plist file to the system location
    $sudo_cmd mv "$user_plist_file" /Library/LaunchDaemons/
    # make sure the daemon launches under proper user/group and that it has access
    # to all files/dirs it needs; then start it
    plist_modify_user_group "$systemwide_servicefile_name" "$systemdaemon_user" "$systemdaemon_group"
    $sudo_cmd chown "0:0" "$systemwide_servicefile_name"
    $sudo_cmd chown -R "$systemdaemon_user_group" "$etc_dir" "$log_dir" "$run_dir"
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
fi

# Set up and start the system-probe service if this version includes support for it
sysprobe_plist_example_file="${etc_dir}/${sysprobe_service_name}.plist.example"
if [ -f "$sysprobe_plist_example_file" ]; then
    printf "${BLUE}\n* Setting up system-probe ($sysprobe_service_name) as a system-wide LaunchDaemon ...\n\n${NC}"
    $sudo_cmd mv "$sysprobe_plist_example_file" "$sysprobe_servicefile_name"
    $sudo_cmd chown "0:0" "$sysprobe_servicefile_name"
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

if [ "$systemdaemon_install" = false ]; then
    printf "${GREEN}
If you ever want to stop the Agent, please use the Datadog Agent App or
the launchctl command. It will start automatically at login.
${NC}"
else
    printf "${GREEN}
If you ever want to stop the Agent, please use the the launchctl command.
The Agent will start automatically at system startup.
${NC}"
fi
