# error-y.tst: yash-specific test of error conditions

test_O -d -e 2 'syntax error kills non-interactive shell'
fi
echo not reached
__IN__

test_O -d -e 2 'syntax error in eval kills non-interactive shell'
eval fi
echo not reached
__IN__

test_o -d 'syntax error in subshell'
(eval fi; echo not reached)
echo $?
__IN__
2
__OUT__

test_o -d 'syntax error spares interactive shell' -i +m
fi
echo $?
__IN__
0
__OUT__

test_o 'redirection error on compound command spares non-interactive shell'
{
if echo not printed 1; then echo not printed 2; fi <_no_such_dir_/foo
printf 'reached\n'
}
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
{
if echo not printed 1; then echo not printed 2; fi <_no_such_dir_/foo
printf 'reached\n'
}
__IN__
reached
__OUT__

test_o 'redirection error on function spares non-interactive shell'
func() { echo not printed; }
{
func <_no_such_dir_/foo
printf 'reached\n'
}
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
{
func <_no_such_dir_/foo
printf 'reached\n'
}
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
echo $?
__IN__
2
__OUT__

test_o -d 'expansion error spares interactive shell' -i +m
unset a
{ echo ${a?}; echo not reached; }
echo $?
__IN__
2
__OUT__

test_o -d -e 128 'unrecoverable read error kills shell'
echo ok so far
exec <&-
echo not reached
__IN__
ok so far
__OUT__

test_o -d 'command not found'
./_no_such_command_
echo $?
__IN__
127
__OUT__

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
{ a=b; printf 'not reached\n'; }
printf 'reached\n'
__IN__
reached
__OUT__

test_o 'assignment error with command name spares interactive shell' -i +m
readonly a=a
{ a=b echo not printed; printf 'not reached\n'; }
printf 'reached\n'
__IN__
reached
__OUT__

###############################################################################

test_Oe -e 2 'built-in short option argument missing'
exec -a
__IN__
exec: the -a option requires an argument
__ERR__

test_Oe -e 2 'built-in long option argument missing'
exec --a
__IN__
exec: the --as option requires an argument
__ERR__

test_Oe -e 2 'built-in short option hyphen'
exec -c-
__IN__
exec: `-c-' is not a valid option
__ERR__
#`

test_Oe -e 2 'built-in invalid short option'
exec -cXaY
__IN__
exec: `-cXaY' is not a valid option
__ERR__
#`

test_Oe -e 2 'built-in invalid long option without argument'
exec --no-such-option
__IN__
exec: `--no-such-option' is not a valid option
__ERR__
#`

test_Oe -e 2 'built-in invalid long option with argument'
exec --no-such=option
__IN__
exec: `--no-such=option' is not a valid option
__ERR__
#`

test_Oe -e 2 'built-in unexpected option argument'
exec --cle=X
__IN__
exec: --cle=X: the --clear option does not take an argument
__ERR__

(
if ! testee -c "set +o posix; command -bv ulimit" >/dev/null; then
    skip="true" # TODO Remove this condition when ulimit is made mandatory
fi

test_O -e 2 'ambiguous long option, exit status and standard output'
ulimit --s
__IN__
)

test_o 'ambiguous long option, standard error'
read --p X 2>&1 | head -n 1
__IN__
read: option `--p' is ambiguous
__OUT__
#`

###############################################################################

# $1 = line no.
# $2 = built-in name
test_special_builtin_syntax_i()
if [ "${posix:+set}" = set ]; then
    testcase "$1" -d \
    "argument syntax error on special built-in $2 spares interactive shell (POSIX)" \
        -i +m 3<<__IN__ 4<<\__OUT__ 5<&-
{ $2 --no-such-option--; echo not reached; }
echo \$?
__IN__
2
__OUT__
else
    testcase "$1" -d \
    "argument syntax error on special built-in $2 spares interactive shell" \
        -i +m 3<<__IN__ 4<<\__OUT__ 5<&-
{ $2 --no-such-option--; echo reached \$?; }
__IN__
reached 2
__OUT__
fi

# $1 = line no.
# $2 = built-in name
test_nonspecial_builtin_syntax() (
    if ! testee -c "set +o posix; command -bv $2" >/dev/null; then
        skip="true"
    fi

    testcase "$1" -d \
    "argument syntax error on non-special built-in $2 spares shell${posix:+" (POSIX)"}" \
        3<<__IN__ 4<<\__OUT__ 5<&-
# -l and -n are mutually exclusive for the kill built-in.
# Four arguments are too many for the test built-in.
$2 -l -n --no-such-option-- -
test \$? -ne 0 && echo reached
__IN__
reached
__OUT__
)

# $1 = line no.
# $2 = built-in name
test_nonspecial_builtin_redirect() {
    testcase "$1" -d \
    "redirection error on non-special built-in $2 spares shell${posix:+" (POSIX)"}" \
        3<<__IN__ 4<<\__OUT__ 5<&-
$2 <_no_such_file_
echo \$?
__IN__
2
__OUT__
}

(
posix="true"

# $1 = line no.
# $2 = built-in name
test_special_builtin_syntax() {
    testcase "$1" -d \
    "argument syntax error on special built-in $2 kills non-interactive shell (POSIX)" \
        3<<__IN__ 4</dev/null 5<&-
$2 --no-such-option--
printf 'not reached\n'
__IN__
}

# $1 = line no.
# $2 = built-in name
test_special_builtin_syntax_s() {
    testcase "$1" -d \
        "argument syntax error on special built-in $2 in subshell (POSIX)" \
        3<<__IN__ 4<<\__OUT__ 5<&-
($2 --no-such-option--; echo not reached)
echo \$?
__IN__
2
__OUT__
}

# No argument syntax error in special built-in colon
# test_special_builtin_syntax   "$LINENO" :
# test_special_builtin_syntax_s "$LINENO" :
# test_special_builtin_syntax_i "$LINENO" :
test_special_builtin_syntax   "$LINENO" .
test_special_builtin_syntax_s "$LINENO" .
test_special_builtin_syntax_i "$LINENO" .
test_special_builtin_syntax   "$LINENO" break
test_special_builtin_syntax_s "$LINENO" break
test_special_builtin_syntax_i "$LINENO" break
test_special_builtin_syntax   "$LINENO" continue
test_special_builtin_syntax_s "$LINENO" continue
test_special_builtin_syntax_i "$LINENO" continue
test_special_builtin_syntax   "$LINENO" eval
test_special_builtin_syntax_s "$LINENO" eval
test_special_builtin_syntax_i "$LINENO" eval
test_special_builtin_syntax   "$LINENO" exec
test_special_builtin_syntax_s "$LINENO" exec
test_special_builtin_syntax_i "$LINENO" exec
test_special_builtin_syntax   "$LINENO" exit
test_special_builtin_syntax_s "$LINENO" exit
test_special_builtin_syntax_i "$LINENO" exit
test_special_builtin_syntax   "$LINENO" export
test_special_builtin_syntax_s "$LINENO" export
test_special_builtin_syntax_i "$LINENO" export
test_special_builtin_syntax   "$LINENO" readonly
test_special_builtin_syntax_s "$LINENO" readonly
test_special_builtin_syntax_i "$LINENO" readonly
test_special_builtin_syntax   "$LINENO" return
test_special_builtin_syntax_s "$LINENO" return
test_special_builtin_syntax_i "$LINENO" return
test_special_builtin_syntax   "$LINENO" set
test_special_builtin_syntax_s "$LINENO" set
test_special_builtin_syntax_i "$LINENO" set
test_special_builtin_syntax   "$LINENO" shift
test_special_builtin_syntax_s "$LINENO" shift
test_special_builtin_syntax_i "$LINENO" shift
test_special_builtin_syntax   "$LINENO" times
test_special_builtin_syntax_s "$LINENO" times
test_special_builtin_syntax_i "$LINENO" times
test_special_builtin_syntax   "$LINENO" trap
test_special_builtin_syntax_s "$LINENO" trap
test_special_builtin_syntax_i "$LINENO" trap
test_special_builtin_syntax   "$LINENO" unset
test_special_builtin_syntax_s "$LINENO" unset
test_special_builtin_syntax_i "$LINENO" unset

test_nonspecial_builtin_syntax "$LINENO" [
test_nonspecial_builtin_syntax "$LINENO" alias
# Non-standard built-in array skipped
# test_nonspecial_builtin_syntax "$LINENO" array
test_nonspecial_builtin_syntax "$LINENO" bg
# Non-standard built-in bindkey skipped
# test_nonspecial_builtin_syntax "$LINENO" bindkey
test_nonspecial_builtin_syntax "$LINENO" cd
test_nonspecial_builtin_syntax "$LINENO" command
# Non-standard built-in complete skipped
# test_nonspecial_builtin_syntax "$LINENO" complete
# Non-standard built-in dirs skipped
# test_nonspecial_builtin_syntax "$LINENO" dirs
# Non-standard built-in disown skipped
# test_nonspecial_builtin_syntax "$LINENO" disown
# No argument syntax error in non-special built-in echo
# test_nonspecial_builtin_syntax "$LINENO" echo
# No argument syntax error in non-special built-in false
# test_nonspecial_builtin_syntax "$LINENO" false
test_nonspecial_builtin_syntax "$LINENO" fc
test_nonspecial_builtin_syntax "$LINENO" fg
test_nonspecial_builtin_syntax "$LINENO" getopts
test_nonspecial_builtin_syntax "$LINENO" hash
# Non-standard built-in help skipped
# test_nonspecial_builtin_syntax "$LINENO" help
# Non-standard built-in history skipped
# test_nonspecial_builtin_syntax "$LINENO" history
test_nonspecial_builtin_syntax "$LINENO" jobs
test_nonspecial_builtin_syntax "$LINENO" kill
# Non-standard built-in popd skipped
# test_nonspecial_builtin_syntax "$LINENO" popd
test_nonspecial_builtin_syntax "$LINENO" printf
# Non-standard built-in pushd skipped
# test_nonspecial_builtin_syntax "$LINENO" pushd
test_nonspecial_builtin_syntax "$LINENO" pwd
test_nonspecial_builtin_syntax "$LINENO" read
# Non-standard built-in suspend skipped
# test_nonspecial_builtin_syntax "$LINENO" suspend
test_nonspecial_builtin_syntax "$LINENO" test
# No argument syntax error in non-special built-in true
# test_nonspecial_builtin_syntax "$LINENO" true
test_nonspecial_builtin_syntax "$LINENO" type
# Non-standard built-in typeset skipped
# test_nonspecial_builtin_syntax "$LINENO" typeset
test_nonspecial_builtin_syntax "$LINENO" ulimit
test_nonspecial_builtin_syntax "$LINENO" umask
test_nonspecial_builtin_syntax "$LINENO" unalias
test_nonspecial_builtin_syntax "$LINENO" wait

test_nonspecial_builtin_redirect "$LINENO" [
test_nonspecial_builtin_redirect "$LINENO" alias
test_nonspecial_builtin_redirect "$LINENO" array
test_nonspecial_builtin_redirect "$LINENO" bg
test_nonspecial_builtin_redirect "$LINENO" bindkey
test_nonspecial_builtin_redirect "$LINENO" cat # example of external command
# test_nonspecial_builtin_redirect "$LINENO" cd # tested in error-p.tst
test_nonspecial_builtin_redirect "$LINENO" command
test_nonspecial_builtin_redirect "$LINENO" complete
test_nonspecial_builtin_redirect "$LINENO" dirs
test_nonspecial_builtin_redirect "$LINENO" disown
test_nonspecial_builtin_redirect "$LINENO" echo
test_nonspecial_builtin_redirect "$LINENO" false
test_nonspecial_builtin_redirect "$LINENO" fc
test_nonspecial_builtin_redirect "$LINENO" fg
test_nonspecial_builtin_redirect "$LINENO" getopts
test_nonspecial_builtin_redirect "$LINENO" hash
test_nonspecial_builtin_redirect "$LINENO" help
test_nonspecial_builtin_redirect "$LINENO" history
test_nonspecial_builtin_redirect "$LINENO" jobs
test_nonspecial_builtin_redirect "$LINENO" kill
test_nonspecial_builtin_redirect "$LINENO" popd
test_nonspecial_builtin_redirect "$LINENO" printf
test_nonspecial_builtin_redirect "$LINENO" pushd
test_nonspecial_builtin_redirect "$LINENO" pwd
test_nonspecial_builtin_redirect "$LINENO" read
test_nonspecial_builtin_redirect "$LINENO" suspend
test_nonspecial_builtin_redirect "$LINENO" test
test_nonspecial_builtin_redirect "$LINENO" true
test_nonspecial_builtin_redirect "$LINENO" type
test_nonspecial_builtin_redirect "$LINENO" typeset
test_nonspecial_builtin_redirect "$LINENO" ulimit
test_nonspecial_builtin_redirect "$LINENO" umask
test_nonspecial_builtin_redirect "$LINENO" unalias
test_nonspecial_builtin_redirect "$LINENO" wait

)

# $1 = line no.
# $2 = built-in name
test_special_builtin_syntax() {
    testcase "$1" -d \
    "argument syntax error on special built-in $2 spares non-interactive shell" \
        3<<__IN__ 4<<\__OUT__ 5<&-
$2 --no-such-option--
echo \$?
__IN__
2
__OUT__
}

# $1 = line no.
# $2 = built-in name
test_special_builtin_syntax_s() {
    testcase "$1" -d \
        "argument syntax error on special built-in $2 in subshell" \
        3<<__IN__ 4<<\__OUT__ 5<&-
($2 --no-such-option--; echo \$?)
__IN__
2
__OUT__
}

# $1 = line no.
# $2 = built-in name
test_special_builtin_redirect() {
    testcase "$1" -d \
        "redirection error on special built-in $2 spares shell" \
        3<<__IN__ 4<<\__OUT__ 5<&-
$2 <_no_such_file_
echo \$?
__IN__
2
__OUT__
}

# $1 = line no.
# $2 = built-in name
test_special_builtin_redirect_i() (
    posix="true"
    testcase "$1" -d \
        "redirection error on special built-in $2 spares interactive shell (POSIX)" \
        -i +m 3<<__IN__ 4<<\__OUT__ 5<&-
{ $2 <_no_such_file_; echo not reached; }
printf 'reached\n'
__IN__
reached
__OUT__
)

# No argument syntax error in special built-in colon
# test_special_builtin_syntax   "$LINENO" :
# test_special_builtin_syntax_s "$LINENO" :
# test_special_builtin_syntax_i "$LINENO" :
test_special_builtin_syntax   "$LINENO" .
test_special_builtin_syntax_s "$LINENO" .
test_special_builtin_syntax_i "$LINENO" .
test_special_builtin_syntax   "$LINENO" break
test_special_builtin_syntax_s "$LINENO" break
test_special_builtin_syntax_i "$LINENO" break
test_special_builtin_syntax   "$LINENO" continue
test_special_builtin_syntax_s "$LINENO" continue
test_special_builtin_syntax_i "$LINENO" continue
test_special_builtin_syntax   "$LINENO" eval
test_special_builtin_syntax_s "$LINENO" eval
test_special_builtin_syntax_i "$LINENO" eval
test_special_builtin_syntax   "$LINENO" exec
test_special_builtin_syntax_s "$LINENO" exec
test_special_builtin_syntax_i "$LINENO" exec
test_special_builtin_syntax   "$LINENO" exit
test_special_builtin_syntax_s "$LINENO" exit
test_special_builtin_syntax_i "$LINENO" exit
test_special_builtin_syntax   "$LINENO" export
test_special_builtin_syntax_s "$LINENO" export
test_special_builtin_syntax_i "$LINENO" export
test_special_builtin_syntax   "$LINENO" readonly
test_special_builtin_syntax_s "$LINENO" readonly
test_special_builtin_syntax_i "$LINENO" readonly
test_special_builtin_syntax   "$LINENO" return
test_special_builtin_syntax_s "$LINENO" return
test_special_builtin_syntax_i "$LINENO" return
test_special_builtin_syntax   "$LINENO" set
test_special_builtin_syntax_s "$LINENO" set
test_special_builtin_syntax_i "$LINENO" set
test_special_builtin_syntax   "$LINENO" shift
test_special_builtin_syntax_s "$LINENO" shift
test_special_builtin_syntax_i "$LINENO" shift
test_special_builtin_syntax   "$LINENO" times
test_special_builtin_syntax_s "$LINENO" times
test_special_builtin_syntax_i "$LINENO" times
test_special_builtin_syntax   "$LINENO" trap
test_special_builtin_syntax_s "$LINENO" trap
test_special_builtin_syntax_i "$LINENO" trap
test_special_builtin_syntax   "$LINENO" unset
test_special_builtin_syntax_s "$LINENO" unset
test_special_builtin_syntax_i "$LINENO" unset

test_nonspecial_builtin_syntax "$LINENO" [
test_nonspecial_builtin_syntax "$LINENO" alias
test_nonspecial_builtin_syntax "$LINENO" array
test_nonspecial_builtin_syntax "$LINENO" bg
test_nonspecial_builtin_syntax "$LINENO" bindkey
test_nonspecial_builtin_syntax "$LINENO" cd
test_nonspecial_builtin_syntax "$LINENO" command
test_nonspecial_builtin_syntax "$LINENO" complete
test_nonspecial_builtin_syntax "$LINENO" dirs
test_nonspecial_builtin_syntax "$LINENO" disown
# No argument syntax error in non-special built-in echo
# test_nonspecial_builtin_syntax "$LINENO" echo
# No argument syntax error in non-special built-in false
# test_nonspecial_builtin_syntax "$LINENO" false
test_nonspecial_builtin_syntax "$LINENO" fc
test_nonspecial_builtin_syntax "$LINENO" fg
test_nonspecial_builtin_syntax "$LINENO" getopts
test_nonspecial_builtin_syntax "$LINENO" hash
test_nonspecial_builtin_syntax "$LINENO" help
test_nonspecial_builtin_syntax "$LINENO" history
test_nonspecial_builtin_syntax "$LINENO" jobs
test_nonspecial_builtin_syntax "$LINENO" kill
test_nonspecial_builtin_syntax "$LINENO" popd
test_nonspecial_builtin_syntax "$LINENO" printf
test_nonspecial_builtin_syntax "$LINENO" pushd
test_nonspecial_builtin_syntax "$LINENO" pwd
test_nonspecial_builtin_syntax "$LINENO" read
test_nonspecial_builtin_syntax "$LINENO" suspend
test_nonspecial_builtin_syntax "$LINENO" test
# No argument syntax error in non-special built-in true
# test_nonspecial_builtin_syntax "$LINENO" true
test_nonspecial_builtin_syntax "$LINENO" type
test_nonspecial_builtin_syntax "$LINENO" typeset
test_nonspecial_builtin_syntax "$LINENO" ulimit
test_nonspecial_builtin_syntax "$LINENO" umask
test_nonspecial_builtin_syntax "$LINENO" unalias
test_nonspecial_builtin_syntax "$LINENO" wait

test_special_builtin_redirect   "$LINENO" :
test_special_builtin_redirect_i "$LINENO" :
test_special_builtin_redirect   "$LINENO" .
test_special_builtin_redirect_i "$LINENO" .
test_special_builtin_redirect   "$LINENO" break
test_special_builtin_redirect_i "$LINENO" break
test_special_builtin_redirect   "$LINENO" continue
test_special_builtin_redirect_i "$LINENO" continue
test_special_builtin_redirect   "$LINENO" eval
test_special_builtin_redirect_i "$LINENO" eval
test_special_builtin_redirect   "$LINENO" exec
test_special_builtin_redirect_i "$LINENO" exec
test_special_builtin_redirect   "$LINENO" exit
test_special_builtin_redirect_i "$LINENO" exit
test_special_builtin_redirect   "$LINENO" export
test_special_builtin_redirect_i "$LINENO" export
test_special_builtin_redirect   "$LINENO" readonly
test_special_builtin_redirect_i "$LINENO" readonly
test_special_builtin_redirect   "$LINENO" return
test_special_builtin_redirect_i "$LINENO" return
test_special_builtin_redirect   "$LINENO" set
test_special_builtin_redirect_i "$LINENO" set
test_special_builtin_redirect   "$LINENO" shift
test_special_builtin_redirect_i "$LINENO" shift
test_special_builtin_redirect   "$LINENO" times
test_special_builtin_redirect_i "$LINENO" times
test_special_builtin_redirect   "$LINENO" trap
test_special_builtin_redirect_i "$LINENO" trap
test_special_builtin_redirect   "$LINENO" unset
test_special_builtin_redirect_i "$LINENO" unset

test_nonspecial_builtin_redirect "$LINENO" [
test_nonspecial_builtin_redirect "$LINENO" alias
test_nonspecial_builtin_redirect "$LINENO" array
test_nonspecial_builtin_redirect "$LINENO" bg
test_nonspecial_builtin_redirect "$LINENO" bindkey
test_nonspecial_builtin_redirect "$LINENO" cat # example of external command
test_nonspecial_builtin_redirect "$LINENO" cd
test_nonspecial_builtin_redirect "$LINENO" command
test_nonspecial_builtin_redirect "$LINENO" complete
test_nonspecial_builtin_redirect "$LINENO" dirs
test_nonspecial_builtin_redirect "$LINENO" disown
test_nonspecial_builtin_redirect "$LINENO" echo
test_nonspecial_builtin_redirect "$LINENO" false
test_nonspecial_builtin_redirect "$LINENO" fc
test_nonspecial_builtin_redirect "$LINENO" fg
test_nonspecial_builtin_redirect "$LINENO" getopts
test_nonspecial_builtin_redirect "$LINENO" hash
test_nonspecial_builtin_redirect "$LINENO" help
test_nonspecial_builtin_redirect "$LINENO" history
test_nonspecial_builtin_redirect "$LINENO" jobs
test_nonspecial_builtin_redirect "$LINENO" kill
test_nonspecial_builtin_redirect "$LINENO" popd
test_nonspecial_builtin_redirect "$LINENO" printf
test_nonspecial_builtin_redirect "$LINENO" pushd
test_nonspecial_builtin_redirect "$LINENO" pwd
test_nonspecial_builtin_redirect "$LINENO" read
test_nonspecial_builtin_redirect "$LINENO" suspend
test_nonspecial_builtin_redirect "$LINENO" test
test_nonspecial_builtin_redirect "$LINENO" true
test_nonspecial_builtin_redirect "$LINENO" type
test_nonspecial_builtin_redirect "$LINENO" typeset
test_nonspecial_builtin_redirect "$LINENO" ulimit
test_nonspecial_builtin_redirect "$LINENO" umask
test_nonspecial_builtin_redirect "$LINENO" unalias
test_nonspecial_builtin_redirect "$LINENO" wait
test_nonspecial_builtin_redirect "$LINENO" ./_no_such_command_

# vim: set ft=sh ts=8 sts=4 sw=4 et:
