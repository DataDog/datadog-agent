#!/bin/sh
# This script is used to allow datadog-updater
# to execute a predefined set of root commands.
# A datadog-root-runner.service executes those commands
# on writes to a fifo file.

error_message=""
assert_unit() {
    unit="$1"
    allowed_chars="abcdefghijklmnopqrstuvwxyz-"
    case $unit in
        "datadog[$allowed_chars]+$")
            return 0
    ;;
        *)
    ;;
    esac
      error_message="invalid unit name: $unit"  
      return 1
}

handle_command() {
    command="$1"
    case "$command" in
        "stop" | "enable" | "disable" | "restart")
            if [ "$#" -ne 2 ]; then
                error_message="missing arguments"
                return 1
            fi
	    unit="$2"
            assert_unit "$unit" || return 1
            if ! systemctl "$command" "$unit"; then
                error_message="failed to $command unit: $unit"
                return 1
            fi
	    return 0
            ;;
        "start")
            if [ "$#" -ne 2 ]; then
                error_message="missing arguments"
                return 1
            fi
	    unit="$2"
            assert_unit "$unit" || return 1
            # --no-block is used to avoid blocking on oneshot executions
            if ! systemctl "$command" "$unit" --no-block ; then
                error_message="failed to $command unit: $unit"
                return 1
            fi
	    return 0
            ;;
        "reload")
            if ! "systemctl daemon-reload"; then
                error_message="failed to reload: $unit"
                return 1
            fi
	    return 0
	    ;;
        "load-systemd")
            if [ "$#" -ne 2 ]; then
                error_message="missing arguments"
                return 1
            fi

            # todo
	    return 0
            ;;
        "rm-systemd")
            if [ "$#" -ne 2 ]; then
                error_message="missing arguments"
                return 1
            fi

            # todo
	    return 0
            ;;
        *)
            error_message="error: Invalid command: $command"
            return 1
            ;;
    esac
}

while true; do
    read -r command < test.fifo
    handle_command "$command"
    case $? in
        0) echo "Success" > out.fifo ;;
        *) echo "error: $error_message" > out.fifo ;;
    esac
    handle_command "$command" > out.fifo
done
