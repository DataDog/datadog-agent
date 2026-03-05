# sigchld2-y.tst: yash-specific test of SIGCHLD handling (2)

posix="true"

. ../signal.sh

signal_action_test_combo "$LINENO" +i +m ignored CHLD

# vim: set ft=sh ts=8 sts=4 sw=4 et:
