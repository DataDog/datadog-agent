# kill1-p.tst: test of the kill built-in for any POSIX-compliant shell, part 1

posix="true"

test_E -e 0 'printing all signal names'
kill -l
__IN__

# $1 = LINENO, $2 = signal number, $3 = signal name w/o SIG
test_printing_signal_name_from_number() {
    testcase "$1" -e 0 "printing signal name $3 from number" \
        3<<__IN__ 4<<__OUT__ 5</dev/null
kill -l $2
__IN__
$3
__OUT__
}

test_printing_signal_name_from_number "$LINENO" 1 HUP
test_printing_signal_name_from_number "$LINENO" 2 INT
test_printing_signal_name_from_number "$LINENO" 3 QUIT
test_printing_signal_name_from_number "$LINENO" 6 ABRT
test_printing_signal_name_from_number "$LINENO" 9 KILL
test_printing_signal_name_from_number "$LINENO" 14 ALRM
test_printing_signal_name_from_number "$LINENO" 15 TERM

# $1 = LINENO, $2 = signal name w/o SIG
test_printing_signal_name_from_exit_status() (
    if sh -c "kill -s $2 \$\$"; then
        skip="true"
    fi
    testcase "$1" -e 0 "printing signal name $2 from exit status" \
        3<<__IN__ 4<<__OUT__ 5</dev/null
sh -c 'kill -s $2 \$\$'
kill -l \$?
__IN__
$2
__OUT__
)

test_printing_signal_name_from_exit_status "$LINENO" HUP
test_printing_signal_name_from_exit_status "$LINENO" INT
test_printing_signal_name_from_exit_status "$LINENO" QUIT
test_printing_signal_name_from_exit_status "$LINENO" ABRT
test_printing_signal_name_from_exit_status "$LINENO" KILL
test_printing_signal_name_from_exit_status "$LINENO" ALRM
test_printing_signal_name_from_exit_status "$LINENO" TERM

test_OE -e TERM 'sending default signal TERM'
kill $$
__IN__

test_OE -e 0 'sending null signal: -s 0'
kill -s 0 $$
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
