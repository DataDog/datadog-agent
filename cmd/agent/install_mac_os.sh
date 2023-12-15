# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# Datadog Agent install script for macOS.
set -e
install_script_version=1.3.1
dmg_file=/tmp/datadog-agent.dmg
dmg_base_url="https://s3.amazonaws.com/dd-agent"
etc_dir=/opt/datadog-agent/etc
log_dir=/opt/datadog-agent/logs
run_dir=/opt/datadog-agent/run
service_name="com.datadoghq.agent"
systemwide_servicefile_name="/Library/LaunchDaemons/${service_name}.plist"

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

function find_latest_patch_version_for() {
    major_minor="$1"
    patch_versions=$(curl "https://s3.amazonaws.com/dd-agent?prefix=datadog-agent-${major_minor}." 2>/dev/null | grep -o "datadog-agent-${major_minor}.[0-9]*-1.dmg")
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
        echo -e "\033[31mFailed to prepare datadog-agent dmg file\033[0m\n"
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
        printf "\033[31mDD_SYSTEMDAEMON_INSTALL set without DD_SYSTEDAEMON_USER_GROUP\033[0m\n"
        exit 1;
    fi
    if ! echo "$systemdaemon_user_group" | grep "^[^:]\+:[^:]\+$" > /dev/null; then
        printf "\033[31mDD_SYSTEMDAEMON_USER_GROUP must be in format UserName:GroupName\033[0m\n"
        exit 1;
    fi
    if echo "$systemdaemon_user_group" | grep ">\|<" > /dev/null; then
        printf "\033[31mDD_SYSTEMDAEMON_USER_GROUP can't contain '>' or '<', because it will be used in XML file\033[0m\n"
        exit 1;
    fi
    systemdaemon_user="$(echo "$systemdaemon_user_group" | awk -F: '{ print $1 }')"
    systemdaemon_group="$(echo "$systemdaemon_user_group" | awk -F: '{ print $2 }')"
    if ! id -u "$systemdaemon_user" >/dev/null 2>&1 ; then
        printf "\033[31mUser $systemdaemon_user not found, can't proceed with installation\033[0m\n"
        exit 1;
    fi
    # dscacheutil -q group output is in form:
    #   name: groupname
    #   password: *
    #   gid: 1001
    #   users: user1 user2
    # so we use `grep` and `awk` to get the group name
    if ! dscacheutil -q group | grep "name:" | awk '{print $2}' | grep -w "$systemdaemon_group" >/dev/null 2>&1; then
        printf "\033[31mGroup $systemdaemon_group not found, can't proceed with installation\033[0m\n"
        exit 1;
    fi
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
  echo -e "\033[33mWarning: DD_AGENT_MAJOR_VERSION not set. Installing Agent version 7 by default.\033[0m"
fi

dmg_version=
if [ "${macos_major_version}" -lt 10 ] || { [ "${macos_major_version}" -eq 10 ] && [ "${macos_minor_version}" -lt 12 ]; }; then
    echo -e "\033[31mDatadog Agent doesn't support macOS < 10.12.\033[0m\n"
    exit 1
elif [ "${macos_major_version}" -eq 10 ] && [ "${macos_minor_version}" -eq 12 ]; then
    if [ -n "${clean_agent_minor_version}" ]; then
        if [ "${agent_minor_version_without_patch}" -gt 34 ]; then
            echo -e "\033[31mmacOS 10.12 only supports Datadog Agent $agent_major_version up to $agent_major_version.34.\033[0m\n"
            exit 1;
        fi
    else
        echo -e "\033[33mWarning: Agent ${agent_major_version}.34.0 is the last supported version for macOS 10.12. Selecting it for installation.\033[0m"
        agent_minor_version_without_patch=34
        agent_patch_version=0
    fi
elif [ "${macos_major_version}" -eq 10 ] && [ "${macos_minor_version}" -eq 13 ]; then
    if [ -n "${clean_agent_minor_version}" ]; then
        if [ "${agent_minor_version_without_patch}" -gt 38 ]; then
            echo -e "\033[31mmacOS 10.13 only supports Datadog Agent $agent_major_version up to $agent_major_version.38.\033[0m\n"
            exit 1;
        fi
    else
        echo -e "\033[33mWarning: Agent ${agent_major_version}.38.2 is the last supported version for macOS 10.13. Selecting it for installation.\033[0m"
        agent_minor_version_without_patch=38
        agent_patch_version=2
    fi
else
    if [ "${agent_major_version}" -eq 6 ]; then
        echo -e "\033[31mThe latest Agent 6 is no longer built for for macOS $macos_full_version. Please invoke again with DD_AGENT_MAJOR_VERSION=7\033[0m\n"
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
            echo -e "\033[33mWarning: Failed to obtain latest patch version for Agent ${agent_major_version}.${agent_minor_version_without_patch}. Defaulting to '0'.\033[0m"
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

if [ -z "$agent_dist_channel" ]; then
    dmg_url="$dmg_base_url/datadog-agent-${dmg_version}.dmg"
else
    dmg_url="$dmg_base_url/$agent_dist_channel/datadog-agent-${dmg_version}.dmg"
fi

if [ "$upgrade" ]; then
    if [ ! -f $etc_dir/datadog.conf ]; then
        printf "\033[31mDD_UPGRADE set but no config was found at $etc_dir/datadog.conf.\033[0m\n"
        exit 1;
    fi
fi

if [ ! "$apikey" ]; then
    # if it's an upgrade, then we will use the transition script
    if [ ! "$upgrade" ]; then
        printf "\033[31mAPI key not available in DD_API_KEY environment variable.\033[0m\n"
        exit 1;
    fi
fi

if [ "$systemdaemon_install" == false ] && [ -f "$systemwide_servicefile_name" ]; then
    printf "\033[31m
$systemwide_servicefile_name exists, suggesting a
systemwide Agent installation is present. Individual users
can't install the Agent when systemwide installation exists.

If no systemwide installation is present or you want to remove it, run:

    sudo launchctl unload -wF $systemwide_servicefile_name
    sudo rm $systemwide_servicefile_name

Then rerun this script to install the Agent for your user account.
\033[0m\n"

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
user_uid=$($cmd_real_user bash -c 'echo "$UID"')
user_plist_file=${install_user_home}/Library/LaunchAgents/${service_name}.plist

# In order to install with the right user
rm -f /tmp/datadog-install-user
echo "$real_user" > /tmp/datadog-install-user

function on_error() {
    printf "\033[31m$ERROR_MESSAGE
It looks like you hit an issue when trying to install the Agent.

Troubleshooting and basic usage information for the Agent are available at:

    https://docs.datadoghq.com/agent/basic_agent_usage/

If you're still having problems, please send an email to support@datadoghq.com
with the contents of ddagent-install.log and we'll do our very best to help you
solve your problem.\n\033[0m\n"
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
    $sudo_cmd chmod 640 $etc_dir/datadog.yaml
}

function import_config() {
    printf "\033[34m\n* Converting old datadog.conf file to new datadog.yaml format\n\033[0m\n"
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
            printf "\033[31m$plist_file already contains <key>$term</key>, please update this script to the latest version\033[0m\n"
            return 1
        fi
    done

    ## to insert user/group into the xml file, we'll find the last "</dict>" occurrence and insert before it
    i_cmd="$(sed_inplace_arg)"
    closing_dict_line=$($sudo_cmd cat "$plist_file" | grep -n "</dict>" | tail -1 | cut -f1 -d:)
    $sudo_cmd sh -c "sed $i_cmd -e \"${closing_dict_line},${closing_dict_line}s|</dict>|<key>$user_parameter</key><string>$user_value</string>\n</dict>|\" -e \"${closing_dict_line},${closing_dict_line}s|</dict>|<key>$group_parameter</key><string>$group_value</string>\n</dict>|\" \"$plist_file\""
}

# # Install the agent
printf "\033[34m\n* Downloading datadog-agent\n\033[0m"
prepare_dmg_file $dmg_file
if ! $sudo_cmd curl --fail --progress-bar "$dmg_url" --output $dmg_file; then
    printf "\033[31mCouldn't download the installer for macOS Agent version ${dmg_version}.\033[0m\n"
    exit 1;
fi
printf "\033[34m\n* Installing datadog-agent, you might be asked for your sudo password...\n\033[0m"
$sudo_cmd hdiutil detach "/Volumes/datadog_agent" >/dev/null 2>&1 || true
printf "\033[34m\n    - Mounting the DMG installer...\n\033[0m"
$sudo_cmd hdiutil attach "$dmg_file" -mountpoint "/Volumes/datadog_agent" >/dev/null
if [ "$systemdaemon_install" != false ] && [ -f "$systemwide_servicefile_name" ]; then
    printf "\033[34m\n    - Stopping systemwide Datadog Agent daemon ...\n\033[0m"
    # we use "|| true" because if the service is not started/loaded, the commands fail
    $sudo_cmd launchctl stop $service_name || true
    if $sudo_cmd launchctl print system/$service_name 2>/dev/null >/dev/null; then
        $sudo_cmd launchctl unload -wF $systemwide_servicefile_name || true
    fi
fi
printf "\033[34m\n    - Unpacking and copying files (this usually takes about a minute) ...\n\033[0m"
cd / && $sudo_cmd /usr/sbin/installer -pkg "`find "/Volumes/datadog_agent" -name \*.pkg 2>/dev/null`" -target / >/dev/null
printf "\033[34m\n    - Unmounting the DMG installer ...\n\033[0m"
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
    printf "\n\033[34m* Restarting the Agent...\n\033[0m\n"
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
          printf "\n\033[33mCould not restart the agent.
You may have to restart it manually using the systray app or the
\"launchctl start $service_name\" command.\n\033[0m\n"
      fi

      $cmd_launchctl start $service_name
    fi
else
    printf "\033[34m\n* A datadog.yaml configuration file already exists. It will not be overwritten.\n\033[0m\n"
fi

# Starting the app
if [ "$systemdaemon_install" = false ]; then
    $cmd_real_user open -a 'Datadog Agent.app'
else
    printf "\033[34m\n* Installing $service_name as a systemwide LaunchDaemon ...\n\n\033[0m"
    # Remove the Agent login item and unload the agent for current user
    # if it is running - it's not running if the script was launched when
    # the GUI was not running for the user (e.g. a run of this script via
    # ssh for user not logged in via GUI).
    if $cmd_launchctl print "gui/$user_uid/$service_name" 1>/dev/null 2>/dev/null; then
        $cmd_real_user osascript -e 'tell application "System Events" to if login item "Datadog Agent" exists then delete login item "Datadog Agent"'
        $cmd_launchctl stop "$service_name"
        $cmd_launchctl unload "$user_plist_file"
    fi
    # move the plist file to the system location
    $sudo_cmd mv "$user_plist_file" /Library/LaunchDaemons/
    # make sure the daemon launches under proper user/group and that it has access
    # to all files/dirs it needs; then start it
    plist_modify_user_group "$systemwide_servicefile_name" "$systemdaemon_user" "$systemdaemon_group"
    $sudo_cmd chown "0:0" "$systemwide_servicefile_name"
    $sudo_cmd chown -R "$systemdaemon_user_group" "$etc_dir" "$log_dir" "$run_dir"
    $sudo_cmd launchctl load -w "$systemwide_servicefile_name"
    $sudo_cmd launchctl kickstart "system/$service_name"
fi

# Agent works, echo some instructions and exit
printf "\033[32m

Your Agent is running properly. It will continue to run in the
background and submit metrics to Datadog.

You can check the agent status using the \"datadog-agent status\" command
or by opening the webui using the \"datadog-agent launch-gui\" command.

\033[0m"

if [ "$systemdaemon_install" = false ]; then
    printf "\033[32m
If you ever want to stop the Agent, please use the Datadog Agent App or
the launchctl command. It will start automatically at login.
\033[0m"
else
    printf "\033[32m
If you ever want to stop the Agent, please use the the launchctl command.
The Agent will start automatically at system startup.
\033[0m"
fi
