# exec-p.tst: test of the exec built-in for any POSIX-compliant shell

posix="true"

(
setup 'set -e'

test_oE 'exec without arguments'
exec
echo reached
__IN__
reached
__OUT__

test_oE 'exec without arguments but -- separator'
exec --
echo $?
__IN__
0
__OUT__

test_Oe 'exec with redirections'
exec >&2 2>/dev/null
echo reached
./_no_such_command_
__IN__
reached
__ERR__

test_Oe -e n 'exec with redirections in grouping'
{ exec 4>&3; } 3>&2
echo foo >&4
{ exec >&3; } 2>/dev/null
__IN__
foo
__ERR__

)

test_oE -e 0 'executing external command'
exec echo foo bar
echo not reached
__IN__
foo bar
__OUT__

test_OE -e 0 'executing external command with option'
exec cat -u /dev/null
__IN__

test_OE -e 0 'executing external command with -- separator'
exec -- cat /dev/null
__IN__

test_OE -e 0 'process ID of executed process'
exec sh -c "[ \$\$ -eq $$ ]"
__IN__

test_oE 'exec in subshell'
(exec echo foo bar)
echo $?
__IN__
foo bar
0
__OUT__

test_O -d -e 127 'executing non-existing command (relative, non-interactive)'
exec ./_no_such_command_
echo not reached
__IN__

test_o -d 'executing non-existing command (relative, interactive)' -i +m
exec ./_no_such_command_
echo $?
__IN__
127
__OUT__

test_x -d -e 0 'redirection error on exec'
command exec <_no_such_file_
status=$?
[ 0 -lt $status ] && [ $status -le 125 ]
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
