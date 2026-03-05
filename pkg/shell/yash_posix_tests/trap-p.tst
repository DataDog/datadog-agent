# trap-p.tst: test of the trap built-in for any POSIX-compliant shell

posix="true"

macos_kill_workaround

test_OE -e USR1 'setting default trap'
trap - USR1
kill -s USR1 $$
__IN__

test_OE -e 0 'setting ignore trap'
trap '' USR1
kill -s USR1 $$
(kill -s USR1 $$)
__IN__

test_oE -e 0 'setting command trap'
trap 'echo trap; echo executed' USR1
kill -s USR1 $$
__IN__
trap
executed
__OUT__

test_OE -e USR1 'resetting to default trap'
trap '' USR1
trap - USR1
kill -s USR1 $$
__IN__

test_oE -e 0 'specifying multiple signals'
trap 'echo trapped' USR1 USR2
kill -s USR1 $$
kill -s USR2 $$
__IN__
trapped
trapped
__OUT__

# $1 = $LINENO, $2 = signal number, $3 = signal name w/o SIG-prefix
test_specifying_signal_by_number() {
    testcase "$1" -e 0 "specifying signal by number ($3)" \
        3<<__IN__ 4<<__OUT__ 5</dev/null
trap 'echo trapped' $2
kill -s $3 \$\$
__IN__
trapped
__OUT__
}

test_specifying_signal_by_number "$LINENO" 1  HUP
test_specifying_signal_by_number "$LINENO" 2  INT
test_specifying_signal_by_number "$LINENO" 3  QUIT
test_specifying_signal_by_number "$LINENO" 6  ABRT
#test_specifying_signal_by_number "$LINENO" 9  KILL
test_specifying_signal_by_number "$LINENO" 14 ALRM
test_specifying_signal_by_number "$LINENO" 15 TERM

test_OE -e INT 'initial numeric operand implies default trap (first operand)'
trap 'echo trapped' 2 QUIT
trap 2 QUIT
kill -s INT $$
__IN__

test_OE -e QUIT 'initial numeric operand implies default trap (second operand)'
trap 'echo trapped' 2 QUIT
trap 2 QUIT
kill -s QUIT $$
__IN__

test_oE -e 0 'setting trap for EXIT (EOF)'
trap 'echo trapped; false' EXIT
echo exiting
__IN__
exiting
trapped
__OUT__

test_oE -e 7 'setting trap for EXIT (exit built-in)'
trap 'echo trapped; (exit 9)' EXIT
exit 7
__IN__
trapped
__OUT__

test_oE -e 0 'exit status of succeeding subshell in signal trap'
trap '(true) && echo ok' INT; kill -s INT $$
__IN__
ok
__OUT__

test_oE -e 0 'exit status of failing subshell in signal trap'
trap '(false) || echo ok' INT; kill -s INT $$
__IN__
ok
__OUT__

test_oE -e n 'exit status of succeeding subshell in EXIT'
trap '(true) && echo ok' EXIT
false
__IN__
ok
__OUT__

test_oE -e 0 'exit status of failing subshell in EXIT'
trap '(false) || echo ok' EXIT
__IN__
ok
__OUT__

test_O -e n 'fatal shell error in signal trap'
trap 'set <_no_such_file_' INT
kill -s INT $$
echo not reached
__IN__

test_O -e n 'fatal shell error in EXIT trap'
trap 'set <_no_such_file_' EXIT
__IN__

test_oE -e 0 '$? is restored after signal trap is executed'
trap 'false' USR1
kill -s USR1 $$
echo $?
__IN__
0
__OUT__

test_E -e 0 'exit status of EXIT trap does not affect exit status of shell'
trap 'false' EXIT
__IN__

test_oE 'trap command is not affected by assignment in same simple command' \
    -c 'foo=1 trap "echo EXIT \$foo" EXIT; foo=2; foo=3 echo $foo'
__IN__
2
EXIT 2
__OUT__

test_oE 'trap command is not affected by assignment for calling function' \
    -c 'f() { echo $foo; }; foo=1 trap "echo EXIT \$foo" EXIT; foo=2; foo=3 f'
__IN__
3
EXIT 2
__OUT__

test_oE 'trap command is not affected by redirections effective when set (1)' \
    -c 'trap "echo foo" EXIT >/dev/null'
__IN__
foo
__OUT__

test_oE 'trap command is not affected by redirections effective when set (2)' \
    -c '{ trap "echo foo" EXIT; } >/dev/null'
__IN__
foo
__OUT__

test_oE 'trap command is not affected by redirections effective when set (3)' \
    -c 'f() { eval "trap \"echo foo\" EXIT"; }; f >/dev/null'
__IN__
foo
__OUT__

test_oE 'trap command is not affected by redirections effective when set (4)' \
    -c 'trap "echo foo" EXIT >/dev/null & wait $!'
__IN__
foo
__OUT__

test_OE 'trap command in subshell is affected by outer redirections' \
    -c '(trap "echo foo" EXIT) >/dev/null'
__IN__

test_oE 'command is evaluated each time trap is executed'
trap X USR1
alias X='echo 1'
kill -s USR1 $$
alias X='echo 2'
kill -s USR1 $$
__IN__
1
2
__OUT__

test_oE 'traps are not handled until foreground job finishes'
trap 'echo trapped' USR1
(
    kill -s USR1 $$
    echo signal sent
)
__IN__
signal sent
trapped
__OUT__

test_oE -e 0 'single trap may be invoked more than once'
trap 'echo trapped' USR1
kill -s USR1 $$
(kill -s USR1 $$)
kill -s USR1 $$
__IN__
trapped
trapped
trapped
__OUT__

test_oE -e 0 'setting new trap in trap'
trap 'trap "echo trapped 2" USR1; echo trapped 1' USR1
kill -s USR1 $$
kill -s USR1 $$
__IN__
trapped 1
trapped 2
__OUT__

test_oE -e 0 'setting new EXIT in subshell in EXIT'
trap '(trap "echo exit" EXIT)' EXIT
__IN__
exit
__OUT__

test_oE -e 0 'printing traps' -e
trap 'echo "a"'"'b'"'\c' USR1
trap >printed_trap
trap - USR1
. ./printed_trap
kill -s USR1 $$
__IN__
abc
__OUT__

test_oE -e 0 'traps are printed even in command substitution' -e
trap 'echo "a"'"'b'"'\c' USR1
printed_trap="$(trap)"
trap - USR1
eval "$printed_trap"
kill -s USR1 $$
__IN__
abc
__OUT__

test_oE -e 0 'without -p, only non-default traps are printed' -e
trap - USR1
trap >printed_trap_1 # should not print USR1
trap 'echo trapped' USR1
. ./printed_trap_1
kill -s USR1 $$
__IN__
trapped
__OUT__

test_OE -e USR1 'with -p, all traps are printed' -e
trap - USR1
trap -p >printed_trap_2 # should print USR1
trap 'echo trapped' USR1
. ./printed_trap_2
kill -s USR1 $$
__IN__

test_oE -e QUIT 'with -p and operands, only specified traps are printed' -e
trap '' INT TERM
trap -p TERM QUIT >printed_trap_3 # should not print INT
trap 'echo INT' INT
trap 'echo TERM' TERM
trap '' QUIT
. ./printed_trap_3 # TERM is ignored again, QUIT is now default
kill -s INT $$ # should print INT
kill -s TERM $$ # should be ignored
kill -s QUIT $$
__IN__
INT
__OUT__

echo 'echo "$@"' > ./-
chmod a+x ./-

test_oE 'setting command trap that starts with hyphen'
PATH=.:$PATH
trap -- '- trapped' USR1
kill -s USR1 $$
__IN__
trapped
__OUT__

test_o -d 'invalid signal does not kill non-interactive shell'
trap '' '' || echo reached
__IN__
reached
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
