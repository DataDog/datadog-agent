# sigcont3-p.tst: test of SIGCONT handling for any POSIX-compliant shell (3)
../checkfg || skip="true" # %REQUIRETTY%

posix="true"

. ../signal.sh

signal_action_test_combo "$LINENO" +i -m default CONT

# vim: set ft=sh ts=8 sts=4 sw=4 et:
