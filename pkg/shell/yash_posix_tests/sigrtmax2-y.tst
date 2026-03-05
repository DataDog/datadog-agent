# sigrtmax2-y.tst: yash-specific test of SIGRTMAX handling (2)

posix="true"
if "$use_valgrind"; then
    skip="true"
fi

. ../signal.sh

signal_action_test_combo "$LINENO" +i +m ignored RTMAX

# vim: set ft=sh ts=8 sts=4 sw=4 et:
