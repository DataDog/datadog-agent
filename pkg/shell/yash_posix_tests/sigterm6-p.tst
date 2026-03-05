# sigterm6-p.tst: test of SIGTERM handling for any POSIX-compliant shell (6)

posix="true"

. ../signal.sh

signal_action_test_combo "$LINENO" -i +m ignored TERM

# vim: set ft=sh ts=8 sts=4 sw=4 et:
