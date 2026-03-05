# kill3-p.tst: test of the kill built-in for any POSIX-compliant shell, part 3

posix="true"

# $1 = LINENO, $2 = signal name w/o SIG, $3 = prefix for $2
test_sending_signal_ignore() {
    testcase "$1" -e 0 "sending signal: $3$2" \
        3<<__IN__ 4</dev/null 5</dev/null
kill $3$2 \$\$
__IN__
}

test_sending_signal_ignore "$LINENO" CHLD '-s '
test_sending_signal_ignore "$LINENO" CONT '-s '
test_sending_signal_ignore "$LINENO" URG  '-s '

test_sending_signal_ignore "$LINENO" CHLD -
test_sending_signal_ignore "$LINENO" CONT -
test_sending_signal_ignore "$LINENO" URG  -

# $1 = LINENO, $2 = signal name w/o SIG, $3 = prefix for $2
test_sending_signal_stop() {
    testcase "$1" -e 0 "sending signal: $3$2" \
        3<<__IN__ 4</dev/null 5</dev/null
(kill $3$2 \$\$; status=\$?; kill -s CONT \$\$; exit \$status)
__IN__
}

test_sending_signal_stop "$LINENO" STOP '-s '
test_sending_signal_stop "$LINENO" TSTP '-s '
test_sending_signal_stop "$LINENO" TTIN '-s '
test_sending_signal_stop "$LINENO" TTOU '-s '

test_sending_signal_stop "$LINENO" STOP -
test_sending_signal_stop "$LINENO" TSTP -
test_sending_signal_stop "$LINENO" TTIN -
test_sending_signal_stop "$LINENO" TTOU -

# $1 = LINENO, $2 = signal name w/o SIG, $3 = signal number
test_sending_signal_num_kill_self() {
    testcase "$1" -e "$2" "sending signal: -$3" \
        3<<__IN__ 4</dev/null 5</dev/null
kill -$3 \$\$
__IN__
}

test_sending_signal_num_kill_self "$LINENO" HUP  1
test_sending_signal_num_kill_self "$LINENO" INT  2
test_sending_signal_num_kill_self "$LINENO" QUIT 3
test_sending_signal_num_kill_self "$LINENO" ABRT 6
test_sending_signal_num_kill_self "$LINENO" KILL 9
test_sending_signal_num_kill_self "$LINENO" ALRM 14
test_sending_signal_num_kill_self "$LINENO" TERM 15

# vim: set ft=sh ts=8 sts=4 sw=4 et:
