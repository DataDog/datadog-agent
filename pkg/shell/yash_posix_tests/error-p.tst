# error-p.tst: test of error conditions for any POSIX-compliant shell

posix="true"

test_O -d -e n 'syntax error kills non-interactive shell'
fi
echo not reached
__IN__

test_O -d -e n 'syntax error in eval kills non-interactive shell'
eval fi
echo not reached
__IN__

test_o -d 'syntax error in subshell'
(eval fi; echo not reached)
[ $? -ne 0 ]
echo $?
__IN__
0
__OUT__

test_o -d 'syntax error spares interactive shell' -i +m
fi
echo reached
__IN__
reached
__OUT__

test_o 'redirection error on compound command spares non-interactive shell'
if echo not printed 1; then echo not printed 2; fi <_no_such_dir_/foo
printf 'reached\n'
__IN__
reached
__OUT__

test_o 'redirection error on compound command in subshell'
(if echo not printed 1; then echo not printed 2; fi <_no_such_dir_/foo
[ \$? -ne 0 ]; printf 'reached %d\n' \$?)
__IN__
reached 0
__OUT__

test_o 'redirection error on compound command spares interactive shell' -i +m
if echo not printed 1; then echo not printed 2; fi <_no_such_dir_/foo
printf 'reached\n'
__IN__
reached
__OUT__

test_o 'redirection error on function spares non-interactive shell'
func() { echo not printed; }
func <_no_such_dir_/foo
printf 'reached\n'
__IN__
reached
__OUT__

test_o 'redirection error on function in subshell'
func() { echo not printed; }
(func <_no_such_dir_/foo; [ \$? -ne 0 ]; printf 'reached %d\n' \$?)
__IN__
reached 0
__OUT__

test_o 'redirection error on function spares interactive shell' -i +m
func() { echo not printed; }
func <_no_such_dir_/foo
printf 'reached\n'
__IN__
reached
__OUT__

test_O -d -e n 'expansion error kills non-interactive shell'
unset a
echo ${a?}
echo not reached
__IN__

test_o -d 'expansion error in subshell'
unset a
(echo ${a?}; echo not reached)
[ $? -ne 0 ]
echo $?
__IN__
0
__OUT__

test_o -d 'expansion error spares interactive shell' -i +m
unset a
echo ${a?}
[ $? -ne 0 ]
echo $?
__IN__
0
__OUT__

test_O -d -e 127 'command not found'
./_no_such_command_
__IN__

###############################################################################

test_O 'assignment error without command kills non-interactive shell'
readonly a=a
a=b
printf 'not reached\n'
__IN__

test_o 'assignment error without command in subshell'
readonly a=a
(a=b; printf 'not reached\n')
[ $? -ne 0 ]
echo $?
__IN__
0
__OUT__

test_o 'assignment error without command spares interactive shell' -i +m
readonly a=a
a=b
printf 'reached\n'
__IN__
reached
__OUT__

# $1 = line no.
# $2 = command name
test_assign() {
    testcase "$1" -d \
        "assignment error on command $2 kills non-interactive shell" \
        3<<__IN__ 4</dev/null 5<&-
readonly a=a
a=b $2
printf 'not reached\n'
__IN__
}

# $1 = line no.
# $2 = command name
test_assign_s() {
    testcase "$1" -d \
        "assignment error on command $2 in subshell" \
        3<<__IN__ 4<<\__OUT__ 5<&-
readonly a=a
(a=b $2; echo not reached)
[ \$? -ne 0 ]
echo \$?
__IN__
0
__OUT__
}

# $1 = line no.
# $2 = command name
test_assign_i() {
    testcase "$1" -d \
        "assignment error on command $2 spares interactive shell" \
        -i +m 3<<__IN__ 4<<\__OUT__ 5<&-
readonly a=a
a=b $2
printf 'reached\n'
__IN__
reached
__OUT__
}

test_assign   "$LINENO" :
test_assign_s "$LINENO" :
test_assign_i "$LINENO" :
test_assign   "$LINENO" .
test_assign_s "$LINENO" .
test_assign_i "$LINENO" .
test_assign   "$LINENO" [
test_assign_s "$LINENO" [
test_assign_i "$LINENO" [
test_assign   "$LINENO" alias
test_assign_s "$LINENO" alias
test_assign_i "$LINENO" alias
test_assign   "$LINENO" array
test_assign_s "$LINENO" array
test_assign_i "$LINENO" array
test_assign   "$LINENO" bg
test_assign_s "$LINENO" bg
test_assign_i "$LINENO" bg
test_assign   "$LINENO" bindkey
test_assign_s "$LINENO" bindkey
test_assign_i "$LINENO" bindkey
test_assign   "$LINENO" break
test_assign_s "$LINENO" break
test_assign_i "$LINENO" break
test_assign   "$LINENO" cat # example of external command
test_assign_s "$LINENO" cat
test_assign_i "$LINENO" cat
test_assign   "$LINENO" cd
test_assign_s "$LINENO" cd
test_assign_i "$LINENO" cd
test_assign   "$LINENO" command
test_assign_s "$LINENO" command
test_assign_i "$LINENO" command
test_assign   "$LINENO" complete
test_assign_s "$LINENO" complete
test_assign_i "$LINENO" complete
test_assign   "$LINENO" continue
test_assign_s "$LINENO" continue
test_assign_i "$LINENO" continue
test_assign   "$LINENO" dirs
test_assign_s "$LINENO" dirs
test_assign_i "$LINENO" dirs
test_assign   "$LINENO" disown
test_assign_s "$LINENO" disown
test_assign_i "$LINENO" disown
test_assign   "$LINENO" echo
test_assign_s "$LINENO" echo
test_assign_i "$LINENO" echo
test_assign   "$LINENO" eval
test_assign_s "$LINENO" eval
test_assign_i "$LINENO" eval
test_assign   "$LINENO" exec
test_assign_s "$LINENO" exec
test_assign_i "$LINENO" exec
test_assign   "$LINENO" exit
test_assign_s "$LINENO" exit
test_assign_i "$LINENO" exit
test_assign   "$LINENO" export
test_assign_s "$LINENO" export
test_assign_i "$LINENO" export
test_assign   "$LINENO" false
test_assign_s "$LINENO" false
test_assign_i "$LINENO" false
test_assign   "$LINENO" fc
test_assign_s "$LINENO" fc
test_assign_i "$LINENO" fc
test_assign   "$LINENO" fg
test_assign_s "$LINENO" fg
test_assign_i "$LINENO" fg
test_assign   "$LINENO" getopts
test_assign_s "$LINENO" getopts
test_assign_i "$LINENO" getopts
test_assign   "$LINENO" hash
test_assign_s "$LINENO" hash
test_assign_i "$LINENO" hash
test_assign   "$LINENO" help
test_assign_s "$LINENO" help
test_assign_i "$LINENO" help
test_assign   "$LINENO" history
test_assign_s "$LINENO" history
test_assign_i "$LINENO" history
test_assign   "$LINENO" jobs
test_assign_s "$LINENO" jobs
test_assign_i "$LINENO" jobs
test_assign   "$LINENO" kill
test_assign_s "$LINENO" kill
test_assign_i "$LINENO" kill
test_assign   "$LINENO" popd
test_assign_s "$LINENO" popd
test_assign_i "$LINENO" popd
test_assign   "$LINENO" printf
test_assign_s "$LINENO" printf
test_assign_i "$LINENO" printf
test_assign   "$LINENO" pushd
test_assign_s "$LINENO" pushd
test_assign_i "$LINENO" pushd
test_assign   "$LINENO" pwd
test_assign_s "$LINENO" pwd
test_assign_i "$LINENO" pwd
test_assign   "$LINENO" read
test_assign_s "$LINENO" read
test_assign_i "$LINENO" read
test_assign   "$LINENO" readonly
test_assign_s "$LINENO" readonly
test_assign_i "$LINENO" readonly
test_assign   "$LINENO" return
test_assign_s "$LINENO" return
test_assign_i "$LINENO" return
test_assign   "$LINENO" set
test_assign_s "$LINENO" set
test_assign_i "$LINENO" set
test_assign   "$LINENO" shift
test_assign_s "$LINENO" shift
test_assign_i "$LINENO" shift
test_assign   "$LINENO" suspend
test_assign_s "$LINENO" suspend
test_assign_i "$LINENO" suspend
test_assign   "$LINENO" test
test_assign_s "$LINENO" test
test_assign_i "$LINENO" test
test_assign   "$LINENO" times
test_assign_s "$LINENO" times
test_assign_i "$LINENO" times
test_assign   "$LINENO" trap
test_assign_s "$LINENO" trap
test_assign_i "$LINENO" trap
test_assign   "$LINENO" true
test_assign_s "$LINENO" true
test_assign_i "$LINENO" true
test_assign   "$LINENO" type
test_assign_s "$LINENO" type
test_assign_i "$LINENO" type
test_assign   "$LINENO" typeset
test_assign_s "$LINENO" typeset
test_assign_i "$LINENO" typeset
test_assign   "$LINENO" ulimit
test_assign_s "$LINENO" ulimit
test_assign_i "$LINENO" ulimit
test_assign   "$LINENO" umask
test_assign_s "$LINENO" umask
test_assign_i "$LINENO" umask
test_assign   "$LINENO" unalias
test_assign_s "$LINENO" unalias
test_assign_i "$LINENO" unalias
test_assign   "$LINENO" unset
test_assign_s "$LINENO" unset
test_assign_i "$LINENO" unset
test_assign   "$LINENO" wait
test_assign_s "$LINENO" wait
test_assign_i "$LINENO" wait
test_assign   "$LINENO" ./_no_such_command_
test_assign_s "$LINENO" ./_no_such_command_
test_assign_i "$LINENO" ./_no_such_command_

test_O 'assignment error in for loop kills non-interactive shell'
readonly a=a
for a in b
do
    printf 'not reached 1\n'
done
printf 'not reached 2\n'
__IN__

test_o 'assignment error in for loop spares interactive shell' -i +m
readonly a=a
for a in b
do
    :
done
printf 'reached\n'
__IN__
reached
__OUT__

# $1 = line no.
# $2 = built-in name
test_special_builtin_redirect() {
    testcase "$1" -d \
        "redirection error on special built-in $2 kills non-interactive shell" \
        3<<__IN__ 4</dev/null 5<&-
$2 <_no_such_file_
printf 'not reached\n'
__IN__
}

# $1 = line no.
# $2 = built-in name
test_special_builtin_redirect_s() {
    testcase "$1" -d \
        "redirection error on special built-in $2 in subshell" \
        3<<__IN__ 4<<\__OUT__ 5<&-
($2 <_no_such_file_; echo not reached)
[ \$? -ne 0 ]
echo \$?
__IN__
0
__OUT__
}

# $1 = line no.
# $2 = built-in name
test_special_builtin_redirect_i() {
    testcase "$1" -d \
        "redirection error on special built-in $2 spares interactive shell" \
        -i +m 3<<__IN__ 4<<\__OUT__ 5<&-
$2 <_no_such_file_
printf 'reached\n'
__IN__
reached
__OUT__
}

test_special_builtin_redirect   "$LINENO" :
test_special_builtin_redirect_s "$LINENO" :
test_special_builtin_redirect_i "$LINENO" :
test_special_builtin_redirect   "$LINENO" .
test_special_builtin_redirect_s "$LINENO" .
test_special_builtin_redirect_i "$LINENO" .
test_special_builtin_redirect   "$LINENO" break
test_special_builtin_redirect_s "$LINENO" break
test_special_builtin_redirect_i "$LINENO" break
test_special_builtin_redirect   "$LINENO" continue
test_special_builtin_redirect_s "$LINENO" continue
test_special_builtin_redirect_i "$LINENO" continue
test_special_builtin_redirect   "$LINENO" eval
test_special_builtin_redirect_s "$LINENO" eval
test_special_builtin_redirect_i "$LINENO" eval
test_special_builtin_redirect   "$LINENO" exec
test_special_builtin_redirect_s "$LINENO" exec
test_special_builtin_redirect_i "$LINENO" exec
test_special_builtin_redirect   "$LINENO" exit
test_special_builtin_redirect_s "$LINENO" exit
test_special_builtin_redirect_i "$LINENO" exit
test_special_builtin_redirect   "$LINENO" export
test_special_builtin_redirect_s "$LINENO" export
test_special_builtin_redirect_i "$LINENO" export
test_special_builtin_redirect   "$LINENO" readonly
test_special_builtin_redirect_s "$LINENO" readonly
test_special_builtin_redirect_i "$LINENO" readonly
test_special_builtin_redirect   "$LINENO" return
test_special_builtin_redirect_s "$LINENO" return
test_special_builtin_redirect_i "$LINENO" return
test_special_builtin_redirect   "$LINENO" set
test_special_builtin_redirect_s "$LINENO" set
test_special_builtin_redirect_i "$LINENO" set
test_special_builtin_redirect   "$LINENO" shift
test_special_builtin_redirect_s "$LINENO" shift
test_special_builtin_redirect_i "$LINENO" shift
test_special_builtin_redirect   "$LINENO" times
test_special_builtin_redirect_s "$LINENO" times
test_special_builtin_redirect_i "$LINENO" times
test_special_builtin_redirect   "$LINENO" trap
test_special_builtin_redirect_s "$LINENO" trap
test_special_builtin_redirect_i "$LINENO" trap
test_special_builtin_redirect   "$LINENO" unset
test_special_builtin_redirect_s "$LINENO" unset
test_special_builtin_redirect_i "$LINENO" unset

test_o 'redirection error on non-special built-in cd spares shell'
cd <_no_such_file_
test $? -ne 0 && echo ok
__IN__
ok
__OUT__

test_o 'redirection error on non-existing command spares shell'
./_no_such_command_ <_no_such_file_
test $? -ne 0 && echo ok
__IN__
ok
__OUT__

test_o 'redirection error without command spares shell'
<_no_such_file_
test $? -ne 0 && echo ok
__IN__
ok
__OUT__

# Command syntax error for special built-ins is not tested here because we can
# not portably cause syntax error since any syntax can be accepted as an
# extension.

# vim: set ft=sh ts=8 sts=4 sw=4 et:
