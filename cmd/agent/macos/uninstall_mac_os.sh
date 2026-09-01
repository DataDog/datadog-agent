#!/usr/bin/env bash
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# Datadog Agent uninstall script for macOS.
# Paired with install_mac_os.sh (Agent 7.79.0+). Removes the system-wide
# components installed by the DMG: LaunchDaemons (agent, sysprobe, data-plane,
# installer), their -exp counterparts, the GUI LaunchAgent, the /Applications
# app, the /opt/datadog-agent state tree, the /opt/datadog-packages code pool,
# and symlinks.
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

ai_usage_desktop_monitor_label="com.datadoghq.ai-usage-agent.desktop-monitor"
old_ai_usage_desktop_monitor_label="com.datadoghq.ai-prompt-logger.desktop-monitor"

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
# The installer daemon is booted out first: it is the thing that would otherwise
# notice the Agent going away and put it back.
$sudo_cmd launchctl bootout system/com.datadoghq.installer 2>/dev/null || true
$sudo_cmd launchctl bootout system/com.datadoghq.agent 2>/dev/null || true
$sudo_cmd launchctl bootout system/com.datadoghq.sysprobe 2>/dev/null || true
$sudo_cmd launchctl bootout system/com.datadoghq.data-plane 2>/dev/null || true

# An uninstall may land while an experiment is running, in which case it is the
# -exp jobs that are up and the stable ones that are unloaded. Both sets are
# booted out unconditionally; booting out a job that is not loaded is a no-op.
$sudo_cmd launchctl bootout system/com.datadoghq.agent-exp 2>/dev/null || true
$sudo_cmd launchctl bootout system/com.datadoghq.sysprobe-exp 2>/dev/null || true
$sudo_cmd launchctl bootout system/com.datadoghq.data-plane-exp 2>/dev/null || true

printf "${BLUE}\n    - Stopping GUI for logged-in users...\n${NC}"
for logged_user in $(who | awk '{print $1}' | sort -u); do
    logged_uid=$(id -u "$logged_user" 2>/dev/null) || continue
    $sudo_cmd launchctl bootout "gui/$logged_uid/com.datadoghq.gui" 2>/dev/null || true
    $sudo_cmd launchctl bootout "gui/$logged_uid/$ai_usage_desktop_monitor_label" 2>/dev/null || true
    $sudo_cmd launchctl bootout "gui/$logged_uid/$old_ai_usage_desktop_monitor_label" 2>/dev/null || true
done
$sudo_cmd pkill -f 'Datadog Agent.app' 2>/dev/null || true
$sudo_cmd pkill -f 'ai-usage-agent-native-host.*--desktop-monitor' 2>/dev/null || true
$sudo_cmd pkill -f 'ai-prompt-logger-native-host.*--desktop-monitor' 2>/dev/null || true

printf "${BLUE}\n    - Removing launchd plists...\n${NC}"
$sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.installer.plist
$sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.agent.plist
$sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.sysprobe.plist
$sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.data-plane.plist
$sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.agent-exp.plist
$sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.sysprobe-exp.plist
$sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.data-plane-exp.plist

# launchd remembers a job's disabled override independently of its definition,
# so a job disabled at some point stays disabled for the next install. Clearing
# the overrides makes a reinstall start from a clean slate.
for label in com.datadoghq.installer com.datadoghq.agent com.datadoghq.sysprobe \
             com.datadoghq.data-plane com.datadoghq.agent-exp \
             com.datadoghq.sysprobe-exp com.datadoghq.data-plane-exp; do
    $sudo_cmd launchctl enable "system/$label" 2>/dev/null || true
done
$sudo_cmd rm -f /Library/LaunchAgents/com.datadoghq.gui.plist
$sudo_cmd rm -f "/Library/LaunchAgents/$ai_usage_desktop_monitor_label.plist"
$sudo_cmd rm -f "/Library/LaunchAgents/$old_ai_usage_desktop_monitor_label.plist"

printf "${BLUE}\n    - Removing application and install directory...\n${NC}"
$sudo_cmd rm -rf "/Applications/Datadog Agent.app"
# /opt/datadog-agent holds the state -- etc, etc-exp, run, logs -- plus the bin
# and embedded symlinks into the pool. Removing it takes the deadline file with
# it, which is what tells a future installer daemon that an experiment is
# running; the pool goes next, so there is nothing left for it to revert to.
$sudo_cmd rm -rf /opt/datadog-agent
# /opt/datadog-packages is the versioned code pool: every installed version,
# plus the stable and experiment links that name two of them. It is a separate
# tree from the state above and would otherwise survive the uninstall entirely,
# leaving several hundred megabytes and, worse, a pool a reinstalled daemon
# would adopt versions from.
$sudo_cmd rm -rf /opt/datadog-packages

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
