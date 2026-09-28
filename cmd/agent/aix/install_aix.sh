#!/bin/ksh
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2016-present Datadog, Inc.

# Datadog Agent 7 install script for AIX.
# Published as https://install.datadoghq.com/scripts/install_aix.sh.
set -e

install_script_version=1.0.0
default_repo_url="https://dd-agent-aix.s3.amazonaws.com"

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

help() {
    cat <<'EOF'
Datadog Agent 7 for AIX

Installs the AIX/ppc64 BFF package and configures it from DD_* environment
variables on first install.

Package selection variables:
  DD_REPO_URL              Package base URL. Defaults to
                           https://dd-agent-aix.s3.amazonaws.com
  DD_AGENT_DIST_CHANNEL    Package channel: stable (default) or beta. Packages
                           are stored below <base>/<channel>/.
  DD_AGENT_VERSION         Exact Agent version, without the package build
                           number (for example, 7.84.0).
  DD_AGENT_MINOR_VERSION   Exact Agent 7 minor and patch version (for example,
                           84.0). Ignored when DD_AGENT_VERSION is set.
  DD_AGENT_BUILD           installp package build number. Defaults to 1.
  DD_BFF_PATH              Use a local BFF instead of downloading one.
  DD_INSECURE              If non-empty, skip TLS certificate validation.

When no version is specified, the script downloads:
  datadog-agent-7-latest.aix.ppc64.bff

Agent configuration variables:
  DD_API_KEY, DD_SITE, DD_HOSTNAME, DD_ENV, DD_TAGS,
  DD_INFRASTRUCTURE_MODE, DD_PROXY_HTTP, DD_PROXY_HTTPS,
  DD_PROXY_NO_PROXY, and DD_INSTALL_ONLY.

The package preserves an existing /etc/datadog-agent/datadog.yaml. On a fresh
install, DD_API_KEY is required to create the configuration and start the Agent.
Without it, the package is installed but the Agent is not started.
EOF
}

fail() {
    printf '%bERROR: %s%b\n' "$RED" "$1" "$NC" >&2
    exit 1
}

case "${1:-}" in
    -h|--help|help)
        help
        exit 0
        ;;
esac
if [ -n "${HELP:-}" ]; then
    help
    exit 0
fi

if [ "$(uname -s)" != "AIX" ]; then
    fail "this install script only supports AIX."
fi

arch=$(uname -p)
case "$arch" in
    powerpc|powerpc64|ppc64)
        package_arch=ppc64
        ;;
    *)
        fail "unsupported AIX architecture '$arch'; only ppc64 is supported."
        ;;
esac

if [ "$(id -u)" = "0" ]; then
    sudo_cmd=
else
    if ! command -v sudo >/dev/null 2>&1; then
        fail "run this script as root, or install sudo."
    fi
    # installp lifecycle scripts consume DD_* variables to render datadog.yaml.
    # -E is therefore required when elevating from a non-root shell.
    sudo_cmd="sudo -E"
fi

downloader=
if [ -z "${DD_BFF_PATH:-}" ]; then
    if command -v curl >/dev/null 2>&1; then
        downloader=curl
    elif command -v wget >/dev/null 2>&1; then
        downloader=wget
    else
        fail "curl or wget is required to download the Agent package."
    fi
fi

download_file() {
    source_url=$1
    destination=$2

    if [ "$downloader" = curl ]; then
        if [ -n "${DD_INSECURE:-}" ]; then
            curl --fail --location --retry 2 --insecure --output "$destination" "$source_url"
        else
            curl --fail --location --retry 2 --output "$destination" "$source_url"
        fi
    elif [ -n "${DD_INSECURE:-}" ]; then
        wget --no-check-certificate --output-document="$destination" "$source_url"
    else
        wget --output-document="$destination" "$source_url"
    fi
}

agent_version=
if [ -n "${DD_AGENT_VERSION:-}" ]; then
    agent_version=$DD_AGENT_VERSION
elif [ -n "${DD_AGENT_MINOR_VERSION:-}" ]; then
    case "$DD_AGENT_MINOR_VERSION" in
        *.*) ;;
        *) fail "DD_AGENT_MINOR_VERSION must include a patch version (for example, 84.0)." ;;
    esac
    agent_version="7.$(printf '%s' "$DD_AGENT_MINOR_VERSION" | tr '~' '-')"
fi

if [ -n "$agent_version" ]; then
    case "$agent_version" in
        7.*) ;;
        *) fail "DD_AGENT_VERSION must select Agent 7 (for example, 7.84.0)." ;;
    esac
    case "$agent_version" in
        *[!A-Za-z0-9._~-]*) fail "DD_AGENT_VERSION contains unsupported characters." ;;
    esac
fi

agent_build=${DD_AGENT_BUILD:-1}
case "$agent_build" in
    ''|*[!0-9]*) fail "DD_AGENT_BUILD must be a positive integer." ;;
    0) fail "DD_AGENT_BUILD must be a positive integer." ;;
esac

agent_dist_channel=${DD_AGENT_DIST_CHANNEL:-stable}
case "$agent_dist_channel" in
    stable|beta) ;;
    *) fail "DD_AGENT_DIST_CHANNEL must be stable or beta." ;;
esac

repo_url=${DD_REPO_URL:-$default_repo_url}
repo_url=${repo_url%/}

if [ -n "$agent_version" ]; then
    bff_name="datadog-agent-${agent_version}-${agent_build}.aix.${package_arch}.bff"
else
    bff_name="datadog-agent-7-latest.aix.${package_arch}.bff"
fi

bff_url="$repo_url/$agent_dist_channel/$bff_name"

# mkdir is atomic. Refusing to proceed when the PID-based path already exists
# avoids following an attacker-controlled symlink on systems without mktemp.
temp_dir="/tmp/datadog-agent-install.$$"
if ! (umask 077 && mkdir "$temp_dir"); then
    fail "could not create secure temporary directory $temp_dir."
fi
trap 'rm -rf "$temp_dir"' EXIT
trap 'exit 1' HUP INT TERM

bff_file="$temp_dir/$bff_name"
installp_log="$temp_dir/installp.log"

if [ -n "${DD_BFF_PATH:-}" ]; then
    if [ ! -f "$DD_BFF_PATH" ]; then
        fail "DD_BFF_PATH is set but does not name a file: $DD_BFF_PATH"
    fi
    printf '%b\n* Using local BFF: %s%b\n' "$BLUE" "$DD_BFF_PATH" "$NC"
    cp "$DD_BFF_PATH" "$bff_file"
else
    printf '%b\n* Downloading %s%b\n' "$BLUE" "$bff_name" "$NC"
    if ! download_file "$bff_url" "$bff_file"; then
        fail "could not download $bff_url"
    fi
fi
chmod 600 "$bff_file"

current_fileset=$(/usr/sbin/installp -ld "$bff_file" 2>&1 | awk 'tolower($0) ~ /datadog-agent/ { print $2; exit }')
if [ -z "$current_fileset" ]; then
    fail "the BFF does not contain the datadog-agent fileset."
fi

installed_fileset=$(/usr/bin/lslpp -l datadog-agent 2>/dev/null | awk '$1 == "datadog-agent" { print $2; exit }' || true)
if [ -n "$installed_fileset" ] && [ "$installed_fileset" = "$current_fileset" ]; then
    install_flags=-acFNXYd
    printf '%b\n* Reinstalling Datadog Agent fileset %s%b\n' "$BLUE" "$current_fileset" "$NC"
else
    install_flags=-aXYgd
    printf '%b\n* Installing Datadog Agent fileset %s%b\n' "$BLUE" "$current_fileset" "$NC"
fi

if ! $sudo_cmd /usr/sbin/installp "$install_flags" "$bff_file" -e "$installp_log" datadog-agent; then
    printf '%bThe Datadog Agent package installation failed.%b\n' "$RED" "$NC" >&2
    if [ -s "$installp_log" ]; then
        printf 'installp log:\n' >&2
        cat "$installp_log" >&2
    fi
    exit 1
fi

install_info=/etc/datadog-agent/install_info
{
    printf '%s\n' '---'
    printf '%s\n' 'install_method:'
    printf '%s\n' '  tool: install_script_aix'
    printf '%s\n' '  tool_version: install_script_aix'
    printf '  installer_version: install_script_aix-%s\n' "$install_script_version"
} | $sudo_cmd tee "$install_info" >/dev/null
$sudo_cmd chown dd-agent:dd-agent "$install_info"
$sudo_cmd chmod 640 "$install_info"

if [ -n "${DD_INSTALL_ONLY:-}" ]; then
    printf '%b\nDatadog Agent %s was installed but not started because DD_INSTALL_ONLY is set.\nStart it with:\n\n    startsrc -g datadog-agent\n%b' "$GREEN" "$current_fileset" "$NC"
elif [ -z "${DD_API_KEY:-}" ] && [ ! -f /etc/datadog-agent/datadog.yaml ]; then
    printf '%b\nDatadog Agent %s was installed but not started because no configuration exists.\nCreate /etc/datadog-agent/datadog.yaml with your API key, then run:\n\n    startsrc -g datadog-agent\n%b' "$YELLOW" "$current_fileset" "$NC"
elif lssrc -s datadog-agent 2>/dev/null | grep -q active; then
    printf '%b\nDatadog Agent %s was installed successfully.\n\nCheck its status with:\n\n    lssrc -g datadog-agent\n\nStop or start all Agent services with:\n\n    stopsrc -g datadog-agent\n    startsrc -g datadog-agent\n%b' "$GREEN" "$current_fileset" "$NC"
else
    printf '%b\nDatadog Agent %s was installed, but the Agent service is not active.\nCheck its status and logs with:\n\n    lssrc -g datadog-agent\n    datadog-agent status\n%b' "$YELLOW" "$current_fileset" "$NC"
fi
