# sigurg3-p.tst: test of SIGURG handling for any POSIX-compliant shell (3)
../checkfg || skip="true" # %REQUIRETTY%

posix="true"

. ../signal.sh

signal_action_test_combo "$LINENO" +i -m default URG

# vim: set ft=sh ts=8 sts=4 sw=4 et:
