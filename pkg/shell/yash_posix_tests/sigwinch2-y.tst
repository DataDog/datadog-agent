# sigwinch2-y.tst: yash-specific test of SIGWINCH handling (2)

posix="true"
if ! kill -WINCH $$ 2>/dev/null; then
    skip="true"
fi

. ../signal.sh

signal_action_test_combo "$LINENO" +i +m ignored WINCH

# vim: set ft=sh ts=8 sts=4 sw=4 et:
