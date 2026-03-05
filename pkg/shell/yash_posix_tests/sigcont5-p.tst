# sigcont5-p.tst: test of SIGCONT handling for any POSIX-compliant shell (5)

posix="true"

. ../signal.sh

signal_action_test_combo "$LINENO" -i +m default CONT

# vim: set ft=sh ts=8 sts=4 sw=4 et:
