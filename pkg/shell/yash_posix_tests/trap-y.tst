# trap-y.tst: yash-specific test of the trap built-in

macos_kill_workaround

test_o 'trap for EXIT is executed just once'
"$TESTEE" -c  'trap "echo EXIT  1" EXIT;  ./_no_such_command_ '
"$TESTEE" -c  'trap "echo EXIT  2" EXIT; (./_no_such_command_)'
"$TESTEE" -ce 'trap "echo EXIT  3" EXIT;  ./_no_such_command_ '
"$TESTEE" -ce 'trap "echo EXIT  4" EXIT; (./_no_such_command_)'
"$TESTEE" -c  'trap "echo EXIT  5" EXIT;  ./_no_such_command_ ; :'
"$TESTEE" -c  'trap "echo EXIT  6" EXIT; (./_no_such_command_); :'
"$TESTEE" -ce 'trap "echo EXIT  7" EXIT;  ./_no_such_command_ ; :'
"$TESTEE" -ce 'trap "echo EXIT  8" EXIT; (./_no_such_command_); :'
"$TESTEE" -c  'trap "echo EXIT  9" EXIT;  ./_no_such_command_ ; (:)'
"$TESTEE" -c  'trap "echo EXIT 10" EXIT; (./_no_such_command_); (:)'
"$TESTEE" -ce 'trap "echo EXIT 11" EXIT;  ./_no_such_command_ ; (:)'
"$TESTEE" -ce 'trap "echo EXIT 12" EXIT; (./_no_such_command_); (:)'
__IN__
EXIT 1
EXIT 2
EXIT 3
EXIT 4
EXIT 5
EXIT 6
EXIT 7
EXIT 8
EXIT 9
EXIT 10
EXIT 11
EXIT 12
__OUT__

{
# In subshell traps other than ignore are cleared.
# Output of the trap built-in reflects it after first trap modification.

test_oE 'setting new trap in subshell'
trap '' USR1
(trap 'echo INT' INT; sh -c 'kill -s USR1 $PPID'; :)
__IN__
__OUT__

(
if "$use_valgrind"; then
    skip="true"
fi

test_oE 'printing after setting in subshell'
trap '' USR1
trap 'echo USR2' USR2
(trap 'echo INT' INT; trap)
__IN__
trap -- 'echo INT' INT
trap -- '' USR1
__OUT__

test_oE 'printing after non-trap command in subshell'
trap '' USR1
trap 'echo USR2' USR2
(echo foo; trap)
__IN__
foo
trap -- '' USR1
trap -- 'echo USR2' USR2
__OUT__

)

test_oE 'ignored signal is still ignored in subshell'
trap '' USR1
(sh -c 'kill -s USR1 $PPID'; echo reached)
__IN__
reached
__OUT__

test_oE 'ignored signal is still ignored after setting for another in subshell'
trap '' USR1
(trap 'echo USR2' USR2; sh -c 'kill -s USR1 $PPID'; echo reached)
__IN__
reached
__OUT__

test_oE 'trapped signal is reset in subshell'
trap 'echo USR1' USR1
(sh -c 'kill -s USR1 $PPID' && echo not reached)
kill -l $?
__IN__
USR1
__OUT__

test_oE 'trapped signal is reset after setting for another in subshell'
trap 'echo USR1' USR1
(trap 'echo USR2' USR2; sh -c 'kill -s USR1 $PPID' && echo not reached)
kill -l $?
__IN__
USR1
__OUT__

}

(
if "$use_valgrind"; then
    skip="true"
fi

test_oE -e 0 'printing all traps (w/o -p)'
trap 'echo "a"'"'b'"'\c' USR1
trap 'echo 1 &
echo 2 ;' USR2
trap
__IN__
trap -- 'echo "a"'\'b\''\c' USR1
trap -- 'echo 1 &
echo 2 ;' USR2
__OUT__

)

test_oE -e 0 'printing all traps (with -p)'
trap 'echo "a"'"'b'"'\c' USR1
trap 'echo USR2' USR2
trap -p | grep 'INT\|QUIT\|KILL\|STOP\|USR'
trap --print | grep 'INT\|QUIT\|KILL\|STOP\|USR'
__IN__
trap -- - INT
trap -- - QUIT
trap -- 'echo "a"'\'b\''\c' USR1
trap -- 'echo USR2' USR2
trap -- - INT
trap -- - QUIT
trap -- 'echo "a"'\'b\''\c' USR1
trap -- 'echo USR2' USR2
__OUT__

test_oE -e 0 'printing specific traps (with -p)'
trap 'echo X' USR1 USR2 HUP
trap 'echo Y' INT QUIT
trap -p QUIT USR1 TERM
__IN__
trap -- 'echo Y' QUIT
trap -- 'echo X' USR1
trap -- - TERM
__OUT__

test_oE -e 0 'specifying signal with SIG-prefix'
trap 'echo trapped' SIGUSR1 && kill -s USR1 $$
__IN__
trapped
__OUT__

test_oE -e 0 'signal name is case-insensitive'
trap 'echo trapped' uSr1 && kill -s USR1 $$
__IN__
trapped
__OUT__

test_oE 'return interrupts trap (but not function outside trap)'
trap 'return; echo not reached' USR1
func() {
    kill -s USR1 $$
    echo reached
}
func
__IN__
reached
__OUT__

test_Oe -e 1 'setting trap for KILL'
trap '' KILL
__IN__
trap: SIGKILL cannot be trapped
__ERR__

test_Oe -e 1 'setting trap for STOP'
trap '' STOP
__IN__
trap: SIGSTOP cannot be trapped
__ERR__

test_Oe -e 2 'invalid option'
trap --no-such-option
__IN__
trap: `--no-such-option' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'missing operand'
trap -
__IN__
trap: this command requires 2 operands
__ERR__

test_Oe -e 1 'invalid signal name'
trap - NOSUCHSIGNAL
__IN__
trap: no such signal `NOSUCHSIGNAL'
__ERR__
#'
#`

test_Oe -e 1 'invalid signal number'
trap -- - -1
__IN__
trap: no such signal `-1'
__ERR__
#'
#`

test_O -d -e 1 'printing to closed stream: printing all traps (w/o -p)'
trap '' USR1
trap >&-
__IN__

test_O -d -e 1 'printing to closed stream: printing all traps (with -p)'
trap '' USR1
trap -p >&-
__IN__

test_O -d -e 1 'printing to closed stream: printing specific traps (with -p)'
trap '' USR1
trap -p USR1 >&-
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
