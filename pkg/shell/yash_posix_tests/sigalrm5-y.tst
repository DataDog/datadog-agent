# sigalrm5-y.tst: yash-specific test of SIGALRM handling (5)

posix="true"

. ../signal.sh

signal_action_test_combo "$LINENO" -i +m default ALRM

# vim: set ft=sh ts=8 sts=4 sw=4 et:
