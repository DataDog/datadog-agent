#!/usr/bin/env bash
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# Datadog Agent uninstall script for macOS.
# Paired with install_mac_os.sh (Agent 7.79.0+). Removes the system-wide
# components installed by the DMG: LaunchDaemons (agent, sysprobe, data-plane),
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
$sudo_cmd launchctl bootout system/com.datadoghq.agent 2>/dev/null || true
$sudo_cmd launchctl bootout system/com.datadoghq.sysprobe 2>/dev/null || true
$sudo_cmd launchctl bootout system/com.datadoghq.data-plane 2>/dev/null || true

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
$sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.agent.plist
$sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.sysprobe.plist
$sudo_cmd rm -f /Library/LaunchDaemons/com.datadoghq.data-plane.plist
$sudo_cmd rm -f /Library/LaunchAgents/com.datadoghq.gui.plist
$sudo_cmd rm -f "/Library/LaunchAgents/$ai_usage_desktop_monitor_label.plist"
$sudo_cmd rm -f "/Library/LaunchAgents/$old_ai_usage_desktop_monitor_label.plist"

# EUDM: Trajectory cleanup. Per-user marker (~/.trajectory/.dd-agent-eudm.state)
# records whether the Agent installed Trajectory (remove it, including per-client
# instrumentation) or it pre-existed (leave it, remove only our config block).
# The marker lives in the user's home, so it is independent of /opt/datadog-agent.
printf "${BLUE}\n    - Removing Agent-managed Trajectory (EUDM)...\n${NC}"
for user_home in /Users/*; do
    [ -d "$user_home" ] || continue
    u=$(basename "$user_home")
    [ "$u" = "Shared" ] && continue
    id -u "$u" >/dev/null 2>&1 || continue

    traj_home="$user_home/.trajectory"
    marker="$traj_home/.dd-agent-eudm.state"
    [ -f "$marker" ] || continue

    traj_owner=$(grep '^trajectory=' "$marker" 2>/dev/null | cut -d= -f2- || true)
    config_state=$(grep '^config=' "$marker" 2>/dev/null | cut -d= -f2- || true)

    if [ "$traj_owner" = "agent" ]; then
        # Agent installed Trajectory for this user: fully remove it. uninstall.sh
        # deregisters the per-client hooks it added (running as the user).
        if [ -f "$traj_home/uninstall.sh" ]; then
            sudo -u "$u" -H /bin/bash "$traj_home/uninstall.sh" -y --remove-config --remove-data 2>/dev/null || true
        fi
        sudo -u "$u" rm -rf "$traj_home" 2>/dev/null || true
    else
        # Trajectory pre-existed: leave it installed; remove only what we added.
        dest="$traj_home/config.defaults.yaml"
        if [ "$config_state" = "created" ]; then
            sudo -u "$u" rm -f "$dest" 2>/dev/null || true
        elif [ -f "$dest" ]; then
            cfg_tmp=$(mktemp /tmp/ddagent-traj-cfg.XXXXXX)
            sed '/# --- BEGIN datadog-agent-eudm ---/,/# --- END datadog-agent-eudm ---/d' "$dest" > "$cfg_tmp" 2>/dev/null || true
            sudo -u "$u" cp "$cfg_tmp" "$dest" 2>/dev/null || true
            rm -f "$cfg_tmp"
        fi
        sudo -u "$u" rm -f "$marker" 2>/dev/null || true
    fi
done

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
