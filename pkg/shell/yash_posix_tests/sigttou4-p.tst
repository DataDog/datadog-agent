# sigttou4-p.tst: test of SIGTTOU handling for any POSIX-compliant shell (4)
../checkfg || skip="true" # %REQUIRETTY%

posix="true"

. ../signal.sh

signal_action_test_combo "$LINENO" +i -m ignored TTOU

# vim: set ft=sh ts=8 sts=4 sw=4 et:
