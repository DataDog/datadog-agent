# sigquit5-p.tst: test of SIGQUIT handling for any POSIX-compliant shell (5)

posix="true"

. ../signal.sh

signal_action_test_combo "$LINENO" -i +m default QUIT

# vim: set ft=sh ts=8 sts=4 sw=4 et:
