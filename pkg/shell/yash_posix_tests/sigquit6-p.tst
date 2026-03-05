# sigquit6-p.tst: test of SIGQUIT handling for any POSIX-compliant shell (6)

posix="true"

. ../signal.sh

signal_action_test_combo "$LINENO" -i +m ignored QUIT

# vim: set ft=sh ts=8 sts=4 sw=4 et:
