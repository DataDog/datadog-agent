# simple-y.tst: yash-specific test of simple commands

setup -d

test_oE 'words are expanded in order of appearance'
a=1-2-3 IFS=
bracket $a ${IFS:= -} $a
__IN__
[1-2-3][][1][2][3]
__OUT__

test_oE 'assignment is exported during and after special built-in execution'
a=1 eval 'sh -c "echo \$a"'
sh -c "echo \$a"
__IN__
1
1
__OUT__

test_oE 'assignment does not persist after function returns'
f() { :; }
a=1
a=2 f
echo $a
__IN__
1
__OUT__

test_oE 'assignment is exported during function execution'
f() { sh -c 'echo $a'; }
a=1 f
__IN__
1
__OUT__

test_O -d 'redirections do not apply to assignments w/o command name'
readonly x=x
x=y 2>/dev/null
__IN__

test_x -e 0 'assignment error aborts redirections w/o command name' -i +m
readonly x=x
x=y >no_such_file
! test -f no_such_file
__IN__

test_o -d 'COMMAND_NOT_FOUND_HANDLER is run when command was not found'
COMMAND_NOT_FOUND_HANDLER=('echo not found' 'echo handled')
./_no_such_command_
__IN__
not found
handled
__OUT__

test_o -d 'COMMAND_NOT_FOUND_HANDLER assignment and command in single command'
COMMAND_NOT_FOUND_HANDLER=('echo not found' 'echo handled') \
./_no_such_command_
__IN__
not found
handled
__OUT__

test_o 'positional parameters in not-found handler'
set -- positional parameters
COMMAND_NOT_FOUND_HANDLER='bracket ! "$@"; set --'
./_no_such_command_ not found 'command arguments'
echo "$@"
__IN__
[!][./_no_such_command_][not][found][command arguments]
positional parameters
__OUT__

test_o 'local variables in not-found handler'
i=out
COMMAND_NOT_FOUND_HANDLER=('typeset i=in' 'echo $i')
./_no_such_command_
echo $i
__IN__
in
out
__OUT__

test_o 'local variable HANDLED is defined empty in not-found handler'
COMMAND_NOT_FOUND_HANDLER=('bracket "${HANDLED-unset}"')
./_no_such_command_
bracket "${HANDLED-unset}"
readonly HANDLED=dummy
COMMAND_NOT_FOUND_HANDLER=('bracket "${HANDLED-unset}"')
./_no_such_command_
bracket "${HANDLED-unset}"
__IN__
[]
[unset]
[]
[dummy]
__OUT__

test_x -e 127 'exit status of not-found command (HANDLED unset)'
COMMAND_NOT_FOUND_HANDLER=('unset HANDLED')
./_no_such_command_
__IN__

test_x -e 127 'exit status of not-found command (HANDLED empty)'
COMMAND_NOT_FOUND_HANDLER=('')
./_no_such_command_
__IN__

test_x -e 29 'exit status of not-found command (HANDLED non-empty)'
COMMAND_NOT_FOUND_HANDLER=('HANDLED=X' '(exit 29)')
./_no_such_command_
__IN__

test_o 'not-found handler is not run recursively'
COMMAND_NOT_FOUND_HANDLER=('echo in' ./_no_such_command_ 'echo out')
./_no_such_command_
__IN__
in
out
__OUT__

(
posix=true

test_O -e 127 'not-found handler is not run in POSIXly-correct mode'
COMMAND_NOT_FOUND_HANDLER='echo not reached'
./_no_such_command_
__IN__

)

# vim: set ft=sh ts=8 sts=4 sw=4 et:
