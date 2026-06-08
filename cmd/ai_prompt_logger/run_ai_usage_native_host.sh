#!/bin/sh
# Launcher for Chrome Native Messaging: run the host with an explicit config path
# (same idea as `agent run -c …` / `system-probe --config=…`).
set -eu
# Avoid CDPATH=…cd (ShellCheck SC1007); subshell keeps CDPATH unset local to each cd.
_bin_dir=$( (unset CDPATH; cd -- "$(dirname "$0")" && pwd) )
_install_root=$( (unset CDPATH; cd -- "$_bin_dir/../.." && pwd) )
exec "$_bin_dir/ai-prompt-logger-native-host" --config="$_install_root/etc/ai_usage_native_host.yaml"
