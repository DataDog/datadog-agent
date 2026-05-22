#!/usr/bin/env bash
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# Datadog Agent uninstall script for macOS.
# Paired with install_mac_os.sh (Agent 7.79.0+). Removes the system-wide
# components installed by the DMG: LaunchDaemons (agent, sysprobe),
# GUI LaunchAgent, /Applications app, /opt/datadog-agent tree, and symlinks.
set -eu

if [ -t 1 ]; then
    RED='\033[31m'
    GREEN='\033[32m'
    BLUE='\033[34m'
    NC='\033[0m'
else
    RED=''
    GREEN=''
    BLUE=''
    NC=''
fi
# Use mktemp so the log path is unpredictable (0600, random suffix). A fixed
# /tmp path would let a local user pre-create it as a symlink and redirect
# root-owned `tee` output into a privileged file.
uninstall_log_file=$(mktemp /tmp/ddagent-uninstall.XXXXXX)
exec > >(tee "$uninstall_log_file") 2>&1

if [ "$(echo "$UID")" = "0" ]; then
    sudo_cmd=''
else
    sudo_cmd='sudo'
fi

function on_error() {
    printf "${RED}
An error occurred during uninstallation. Some components may still be
present on the system. See the log at:

    %s
${NC}\n" "$uninstall_log_file"
}
trap on_error ERR

printf "${BLUE}* Uninstalling Datadog Agent, you might be asked for your sudo password...\n${NC}"

printf "${BLUE}\n    - Stopping system services...\n${NC}"
$sudo_cmd launchctl bootout system/com.datadoghq.agent 2>/dev/null || true
$sudo_cmd launchctl bootout system/com.datadoghq.sysprobe 2>/dev/null || true

printf "${BLUE}\n    - Stopping GUI for logged-in users...\n${NC}"
for logged_user in $(who | awk '{print $1}' | sort -u); do
    logged_uid=$(id -u "$logged_user" 2>/dev/null) || continue
    $sudo_cmd launchctl bootout "gui/$logged_uid/com.datadoghq.gui" 2>/dev/null || true
done
$sudo_cmd pkill -f 'Datadog Agent.app' 2>/dev/null || true

printf "${BLUE}\n    - Removing launchd plists...\n${NC}"
$sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.agent.plist
$sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.sysprobe.plist
$sudo_cmd rm -f /Library/LaunchAgents/com.datadoghq.gui.plist

printf "${BLUE}\n    - Removing application and install directory...\n${NC}"
$sudo_cmd rm -rf "/Applications/Datadog Agent.app"
$sudo_cmd rm -rf /opt/datadog-agent

printf "${BLUE}\n    - Removing symlinks and staging data...\n${NC}"
$sudo_cmd rm -f /usr/local/bin/datadog-agent
# /var/log/datadog is a symlink to /opt/datadog-agent/logs created by preinst.
$sudo_cmd rm -f /var/log/datadog
# Staging dir may be left behind by an interrupted install (normally cleaned
# by postinst or install_mac_os.sh's EXIT trap).
$sudo_cmd rm -rf /private/var/root/datadog-install

printf "${GREEN}

Datadog Agent has been uninstalled.
${NC}\n"
