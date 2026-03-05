# exit-p.tst: test of the exit built-in for any POSIX-compliant shell

posix="true"

test_OE -e 0 'exiting with 0'
false
exit 0
__IN__

test_OE -e 17 'exiting with 17'
exit 17
__IN__

test_OE -e 19 'exiting with 19 in subshell'
(exit 19)
__IN__

test_OE -e 0 'default exit status without previous command'
exit
__IN__

test_OE -e 0 'default exit status with previous succeeding command'
true
exit
__IN__

test_OE -e 5 'default exit status with previous failing command'
(exit 5)
exit
__IN__

test_OE -e 3 'default exit status in subshell'
(exit 3)
(exit)
__IN__

test_oE -e 19 'exiting with EXIT trap'
trap 'echo TRAP' EXIT
exit 19
__IN__
TRAP
__OUT__

test_OE -e 1 'exit status with EXIT trap'
trap '(exit 2)' EXIT
(exit 1)
exit
__IN__

test_OE -e 0 'exiting from EXIT trap with 0'
trap 'exit 0' EXIT
exit 1
__IN__

test_OE -e 7 'exiting from EXIT trap with 7'
trap 'exit 7' EXIT
exit 1
__IN__

test_OE -e 2 'default exit status in EXIT trap in exiting with default'
trap exit EXIT
(exit 2)
exit
__IN__

test_OE -e 2 \
    'default exit status with previous command in trap in exiting with default'
trap '(exit 1); exit' EXIT
(exit 2)
exit
__IN__

# POSIX says the exit status in this case should be "the value (of the special
# parameter '?') it had immediately preceding the trap action." Many shells
# including yash interpret it as the exit status of "exit" rather than "trap."
test_OE -e 1 'default exit status in EXIT trap in exiting with 1'
trap exit EXIT
exit 1
__IN__

macos_kill_workaround

test_OE -e 3 'exit from signal trap with 3'
trap '(exit 2); exit 3' INT
(exit 1)
kill -INT $$
__IN__

test_OE -e 0 'default exit status in signal trap'
trap '(exit 2); exit' INT
(exit 1)
kill -INT $$
__IN__

test_oE -e 0 'default exit status in subshell in signal trap'
trap '((exit 2); exit); echo $?' INT
(exit 1)
kill -INT $$
__IN__
2
__OUT__

(
# The test cases below are applicable only if the shell uses exit statuses
# greater than 256 for commands terminated by signals.
if
testee -s <<'__END__'
sh -c 'kill $$'
test $? -le 256
__END__
then
    skip=true
fi

test_o 'exit built-in kills shell according to exit status (TERM)'
"$TESTEE" -s <<'__END__'
# This `sh` kills itself with SIGTERM
sh -c 'kill $$'
# Now the exit status should be a value greater than 256
# indicating that the previous command was terminated by SIGTERM.
# The exit built-in should kill the shell with the same signal
# to propagate the exit status.
exit
__END__
exit_status=$?
test "$exit_status" -gt 256 ||
echo "exit status $exit_status is not greater than 256"
kill -l "$exit_status"
__IN__
TERM
__OUT__

test_o 'exit built-in kills shell according to exit status (KILL)'
"$TESTEE" -s <<'__END__'
# This `sh` kills itself with SIGKILL
sh -c 'kill -s KILL $$'
# Now the exit status should be a value greater than 256
# indicating that the previous command was terminated by SIGKILL.
# The exit built-in should kill the shell with the same signal
# to propagate the exit status.
exit
__END__
exit_status=$?
test "$exit_status" -gt 256 ||
echo "exit status $exit_status is not greater than 256"
kill -l "$exit_status"
__IN__
KILL
__OUT__

test_o 'exit built-in kills subshell according to exit status'
(
# This `sh` kills itself with SIGTERM
sh -c 'kill $$'
# Now the exit status should be a value greater than 256
# indicating that the previous command was terminated by SIGTERM.
# The exit built-in should kill the subshell with the same signal
# to propagate the exit status.
exit
)
exit_status=$?
test "$exit_status" -gt 256 ||
echo "exit status $exit_status is not greater than 256"
kill -l "$exit_status"
__IN__
TERM
__OUT__

test_o 'shell kills itself according to final exit status'
"$TESTEE" -s <<'__END__'
# This `sh` kills itself with SIGTERM
sh -c 'kill $$'
# Now the exit status should be a value greater than 256
# indicating that the previous command was terminated by SIGTERM.
# When reaching the end of the script, the shell should kill
# itself with the same signal to propagate the exit status.
__END__
exit_status=$?
test "$exit_status" -gt 256 ||
echo "exit status $exit_status is not greater than 256"
kill -l "$exit_status"
__IN__
TERM
__OUT__

test_o 'subshell kills itself according to final exit status'
(
# This dummy trap suppresses possible auto-exec optimization
trap 'echo foo' TERM
# This `sh` kills itself with SIGTERM
sh -c 'kill $$'
# Now the exit status should be a value greater than 256
# indicating that the previous command was terminated by SIGTERM.
# When reaching the end of the script, the subshell should kill
# itself with the same signal to propagate the exit status.
)
exit_status=$?
test "$exit_status" -gt 256 ||
echo "exit status $exit_status is not greater than 256"
kill -l "$exit_status"
__IN__
TERM
__OUT__

)

test_OE -e 56 'separator preceding operand'
exit -- 56
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
