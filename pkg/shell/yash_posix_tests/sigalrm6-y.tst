# sigalrm6-y.tst: yash-specific test of SIGALRM handling (6)

posix="true"

. ../signal.sh

signal_action_test_combo "$LINENO" -i +m ignored ALRM

# vim: set ft=sh ts=8 sts=4 sw=4 et:
