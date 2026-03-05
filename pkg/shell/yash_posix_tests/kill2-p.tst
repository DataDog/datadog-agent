# kill2-p.tst: test of the kill built-in for any POSIX-compliant shell, part 2

posix="true"

# $1 = LINENO, $2 = signal name w/o SIG, $3 = prefix for $2
test_sending_signal_kill() {
    testcase "$1" -e "$2" "sending signal: $3$2" \
        3<<__IN__ 4</dev/null 5</dev/null
kill $3$2 \$\$
__IN__
}

test_sending_signal_kill "$LINENO" ABRT '-s '
test_sending_signal_kill "$LINENO" ALRM '-s '
test_sending_signal_kill "$LINENO" BUS  '-s '
test_sending_signal_kill "$LINENO" FPE  '-s '
test_sending_signal_kill "$LINENO" HUP  '-s '
test_sending_signal_kill "$LINENO" ILL  '-s '
test_sending_signal_kill "$LINENO" INT  '-s '
test_sending_signal_kill "$LINENO" KILL '-s '
test_sending_signal_kill "$LINENO" PIPE '-s '
test_sending_signal_kill "$LINENO" QUIT '-s '
test_sending_signal_kill "$LINENO" SEGV '-s '
test_sending_signal_kill "$LINENO" TERM '-s '
test_sending_signal_kill "$LINENO" USR1 '-s '
test_sending_signal_kill "$LINENO" USR2 '-s '

test_sending_signal_kill "$LINENO" ABRT -
test_sending_signal_kill "$LINENO" ALRM -
test_sending_signal_kill "$LINENO" BUS  -
test_sending_signal_kill "$LINENO" FPE  -
test_sending_signal_kill "$LINENO" HUP  -
test_sending_signal_kill "$LINENO" ILL  -
test_sending_signal_kill "$LINENO" INT  -
test_sending_signal_kill "$LINENO" KILL -
test_sending_signal_kill "$LINENO" PIPE -
test_sending_signal_kill "$LINENO" QUIT -
test_sending_signal_kill "$LINENO" SEGV -
test_sending_signal_kill "$LINENO" TERM -
test_sending_signal_kill "$LINENO" USR1 -
test_sending_signal_kill "$LINENO" USR2 -

# vim: set ft=sh ts=8 sts=4 sw=4 et:
