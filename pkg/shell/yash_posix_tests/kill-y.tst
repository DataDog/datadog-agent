# kill-y.tst: yash-specific test of the kill built-in

# $1 = LINENO, $2 = signal name w/o SIG
test_printing_signal_name_from_name() {
    testcase "$1" -e 0 "printing signal name $2 from name" \
        3<<__IN__ 4<<__OUT__ 5</dev/null
kill -l $2
__IN__
$2
__OUT__
}

test_printing_signal_name_from_name "$LINENO" INT
test_printing_signal_name_from_name "$LINENO" TERM

test_E -e 0 'printing signal description from number'
kill -v 9
__IN__

test_E -e 0 'printing signal description from name'
kill -v KILL
__IN__

# $1 = LINENO, $2 = signal name w/o SIG, $3 = prefix for $2
test_sending_signal_kill() {
    testcase "$1" -e "$2" "sending signal: $3$2" \
        3<<__IN__ 4</dev/null 5</dev/null
kill $3$2 \$\$
__IN__
}

test_sending_signal_kill "$LINENO" USR1 '-n '
test_sending_signal_kill "$LINENO" USR2 '-n '

# $1 = LINENO, $2 = signal name w/o SIG, $3 = signal option for the built-in
test_sending_signal_num_kill_self() {
    testcase "$1" -e "$2" "sending signal: $3" \
        3<<__IN__ 4</dev/null 5</dev/null
kill $3 \$\$
__IN__
}

test_sending_signal_num_kill_self "$LINENO" ALRM '-s 14'
test_sending_signal_num_kill_self "$LINENO" TERM '-s 15'

test_sending_signal_num_kill_self "$LINENO" ALRM '-n 14'
test_sending_signal_num_kill_self "$LINENO" TERM '-n 15'

test_oE 'various syntaxes of kill'
kill -s CHLD $$ $$
echo kill 1 $?
kill -n CHLD $$ $$
echo kill 2 $?
kill -s 0 $$ $$
echo kill 3 $?
kill -n 0 $$ $$
echo kill 4 $?
kill -sCHLD $$ $$
echo kill 5 $?
kill -nCHLD $$ $$
echo kill 6 $?
kill -s0 $$ $$
echo kill 7 $?
kill -n0 $$ $$
echo kill 8 $?
kill -CHLD $$ $$
echo kill 9 $?
kill -0 $$ $$
echo kill 10 $?
kill -s CHLD -- $$ $$
echo kill 11 $?
kill -n CHLD -- $$ $$
echo kill 12 $?
kill -s 0 -- $$ $$
echo kill 13 $?
kill -n 0 -- $$ $$
echo kill 14 $?
kill -sCHLD -- $$ $$
echo kill 15 $?
kill -nCHLD -- $$ $$
echo kill 16 $?
kill -s0 -- $$ $$
echo kill 17 $?
kill -n0 -- $$ $$
echo kill 18 $?
kill -CHLD -- $$ $$
echo kill 19 $?
kill -0 -- $$ $$
echo kill 20 $?
kill -l >/dev/null
echo kill 21 $?
kill -l -v >/dev/null
echo kill 22 $?
kill -v -l >/dev/null
echo kill 23 $?
kill -lv >/dev/null
echo kill 24 $?
kill -vl >/dev/null
echo kill 25 $?
kill -v >/dev/null
echo kill 26 $?
kill -l -- 3 9 15
echo kill 27 $?
kill -lv -- 3 9 15 >/dev/null
echo kill 28 $?
kill -s chld $$ $$
echo kill 29 $?
kill -schld $$ $$
echo kill 30 $?
kill -s SIGCHLD $$ $$
echo kill 31 $?
kill -sSIGCHLD $$ $$
echo kill 32 $?
kill -s sigchld $$ $$
echo kill 33 $?
kill -ssigchld $$ $$
echo kill 34 $?
kill -SIGCHLD $$ $$
echo kill 35 $?
kill -SiGcHlD $$ $$
echo kill 36 $?
__IN__
kill 1 0
kill 2 0
kill 3 0
kill 4 0
kill 5 0
kill 6 0
kill 7 0
kill 8 0
kill 9 0
kill 10 0
kill 11 0
kill 12 0
kill 13 0
kill 14 0
kill 15 0
kill 16 0
kill 17 0
kill 18 0
kill 19 0
kill 20 0
kill 21 0
kill 22 0
kill 23 0
kill 24 0
kill 25 0
kill 26 0
QUIT
KILL
TERM
kill 27 0
kill 28 0
kill 29 0
kill 30 0
kill 31 0
kill 32 0
kill 33 0
kill 34 0
kill 35 0
kill 36 0
__OUT__

test_Oe -e 1 'invalid option'
kill --no-such-option
__IN__
kill: no such signal `-NO-SUCH-OPTION'
__ERR__
#'
#`

test_Oe -e 2 'specifying -l and -n both'
kill -l -n 0
__IN__
kill: the -n option cannot be used with the -l option
__ERR__

test_Oe -e 2 'specifying -l and -s both'
kill -l -s INT
__IN__
kill: the -s option cannot be used with the -l option
__ERR__

test_Oe -e 2 'missing operand'
kill
__IN__
kill: this command requires an operand
__ERR__

test_Oe -e 1 'printing name of unknown signal'
kill -l 0
__IN__
kill: no such signal `0'
__ERR__
#'
#`

test_Oe -e 1 'sending signal to unknown job'
kill %100
__IN__
kill: no such job `%100'
__ERR__
#'
#`

test_O -d -e 1 'printing to closed stream (no operand)'
kill -l >&-
__IN__

test_O -d -e 1 'printing to closed stream (with operand)'
kill -l 1 >&-
__IN__

test_Oe -e 2 'signal name must be specified w/o SIG (POSIX)' --posix
kill -s SIGTERM $$
__IN__
kill: SIGTERM: the signal name must be specified without `SIG'
__ERR__
#'
#`

(
if ! testee --version --verbose | grep -Fqx ' * help'; then
    skip="true"
fi

test_oE -e 0 'help'
kill --help
__IN__
kill: send a signal to processes

Syntax:
	kill [-signal|-s signal|-n number] process...
	kill -l [-v] [number...]

Try `man yash' for details.
__OUT__
#'
#`

)

test_Oe -e 1 'no help in POSIX mode' --posix
kill --help
__IN__
kill: no such signal `-HELP'
__ERR__
#'
#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
