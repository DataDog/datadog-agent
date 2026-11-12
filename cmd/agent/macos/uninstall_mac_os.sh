#!/usr/bin/env bash
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# Datadog Agent uninstall script for macOS.
# Paired with install_mac_os.sh (Agent 7.79.0+). Removes the system-wide
# components installed by the DMG: LaunchDaemons (procmgr, agent, sysprobe,
# data-plane, installer, and a configuration experiment's -exp jobs), GUI
# LaunchAgent, /Applications app, /opt/datadog-agent and /opt/datadog-packages
# trees, and symlinks.
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

# uninstall_without_installer removes everything the .dmg installed, for versions whose installer
# cannot do it: versions before the installer have none, and one whose pre-remove failed may have
# left things behind. Every step is a no-op for what is already gone.
uninstall_without_installer() {
    printf "${BLUE}\n    - Stopping system services...\n${NC}"
    # A configuration experiment's jobs first, so none outlives the configuration it reads.
    $sudo_cmd launchctl bootout system/com.datadoghq.installer-exp 2>/dev/null || true
    $sudo_cmd launchctl bootout system/com.datadoghq.agent-exp 2>/dev/null || true
    $sudo_cmd launchctl bootout system/com.datadoghq.sysprobe-exp 2>/dev/null || true
    $sudo_cmd launchctl bootout system/com.datadoghq.data-plane-exp 2>/dev/null || true
    $sudo_cmd launchctl bootout system/com.datadoghq.procmgr 2>/dev/null || true
    $sudo_cmd launchctl bootout system/com.datadoghq.agent 2>/dev/null || true
    $sudo_cmd launchctl bootout system/com.datadoghq.sysprobe 2>/dev/null || true
    $sudo_cmd launchctl bootout system/com.datadoghq.data-plane 2>/dev/null || true
    $sudo_cmd launchctl bootout system/com.datadoghq.installer 2>/dev/null || true
    # bootout returns before a job has left the domain. Wait up to 30 seconds for each, as the
    # installer's pre-remove does, so the Agent is stopped before its files are removed.
    for label in com.datadoghq.installer-exp com.datadoghq.agent-exp com.datadoghq.sysprobe-exp com.datadoghq.data-plane-exp com.datadoghq.procmgr com.datadoghq.agent com.datadoghq.sysprobe com.datadoghq.data-plane com.datadoghq.installer; do
        attempts=0
        while [ $attempts -lt 30 ] && $sudo_cmd launchctl print "system/$label" >/dev/null 2>&1; do
            sleep 1
            attempts=$((attempts + 1))
        done
    done

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
    $sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.procmgr.plist
    $sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.agent.plist
    $sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.sysprobe.plist
    $sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.data-plane.plist
    $sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.installer.plist
    $sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.installer-exp.plist
    $sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.agent-exp.plist
    $sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.sysprobe-exp.plist
    $sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.data-plane-exp.plist
    $sudo_cmd rm -f /Library/LaunchAgents/com.datadoghq.gui.plist
    $sudo_cmd rm -f "/Library/LaunchAgents/$ai_usage_desktop_monitor_label.plist"
    $sudo_cmd rm -f "/Library/LaunchAgents/$old_ai_usage_desktop_monitor_label.plist"

    printf "${BLUE}\n    - Removing application and install directory...\n${NC}"
    $sudo_cmd rm -rf "/Applications/Datadog Agent.app"
    $sudo_cmd rm -rf /opt/datadog-agent
    # The installer daemon's own state and logs, and the package entry it reports the Agent from.
    $sudo_cmd rm -rf /opt/datadog-packages

    printf "${BLUE}\n    - Removing symlinks and staging data...\n${NC}"
    $sudo_cmd rm -f /usr/local/bin/datadog-agent
    $sudo_cmd rm -f /usr/local/bin/datadog-installer
    # /var/log/datadog is a symlink to /opt/datadog-agent/logs created by preinst.
    $sudo_cmd rm -f /var/log/datadog
    # Staging dir may be left behind by an interrupted install (normally cleaned
    # by postinst or install_mac_os.sh's EXIT trap).
    $sudo_cmd rm -rf /private/var/root/datadog-install
}

# Hand the uninstall to the installed version, as the .dmg preinstall script hands it the teardown
# on upgrade: it knows what it installed, and removes all of it. Each job's bootout is bounded at
# 30 seconds. See preRemoveDatadogAgent in pkg/fleet/installer/packages/datadog_agent_darwin.go
installer_bin=/opt/datadog-agent/embedded/bin/installer
if $sudo_cmd test -x "$installer_bin" && $sudo_cmd "$installer_bin" is-prerm-supported >/dev/null 2>&1; then
    printf "${BLUE}\n    - Running the installed version's pre-remove...\n${NC}"
    if ! $sudo_cmd "$installer_bin" prerm datadog-agent dmg; then
        printf "${RED}    The installed version's pre-remove failed; removing the Agent directly.\n${NC}"
        uninstall_without_installer
    elif $sudo_cmd test -e /opt/datadog-agent; then
        # Earlier versions' pre-remove only takes down the LaunchDaemons.
        uninstall_without_installer
    fi
else
    uninstall_without_installer
fi

printf "${GREEN}

Datadog Agent has been uninstalled.
${NC}\n"
