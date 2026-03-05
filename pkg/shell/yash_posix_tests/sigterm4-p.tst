# sigterm4-p.tst: test of SIGTERM handling for any POSIX-compliant shell (4)
../checkfg || skip="true" # %REQUIRETTY%

posix="true"

. ../signal.sh

signal_action_test_combo "$LINENO" +i -m ignored TERM

# vim: set ft=sh ts=8 sts=4 sw=4 et:
