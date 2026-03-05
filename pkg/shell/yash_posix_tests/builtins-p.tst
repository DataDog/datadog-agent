# builtins-p.tst: test of built-ins' attributes for any POSIX-compliant shell

posix="true"

##### Special built-ins

test_o 'assignment on special built-in colon is persistent'
a=a
a=b :
echo $a
__IN__
b
__OUT__

test_o 'assignment on special built-in dot is persistent'
a=a
a=b . /dev/null
echo $a
__IN__
b
__OUT__

test_o 'assignment on special built-in break is persistent'
a=a
for i in 1; do
    a=b break
done
echo $a
__IN__
b
__OUT__

test_o 'assignment on special built-in continue is persistent'
a=a
for i in 1; do
    a=b continue
done
echo $a
__IN__
b
__OUT__

test_o 'assignment on special built-in eval is persistent'
a=a
a=b eval ''
echo $a
__IN__
b
__OUT__

test_o 'assignment on special built-in exec is persistent'
a=a
a=b exec
echo $a
__IN__
b
__OUT__

#test_o 'assignment on special built-in exit is persistent'

test_o 'assignment on special built-in export is persistent'
a=a
a=b export c=c
echo $a
__IN__
b
__OUT__

test_o 'assignment on special built-in readonly is persistent'
a=a
a=b readonly c=c
echo $a
__IN__
b
__OUT__

test_o 'assignment on special built-in return is persistent'
f() { a=b return; }
a=a
f
echo $a
__IN__
b
__OUT__

test_o 'assignment on special built-in set is persistent'
a=a
a=b set ''
echo $a
__IN__
b
__OUT__

test_o 'assignment on special built-in shift is persistent'
a=a
a=b shift 0
echo $a
__IN__
b
__OUT__

test_o 'assignment on special built-in times is persistent'
a=a
a=b times >/dev/null
echo $a
__IN__
b
__OUT__

test_o 'assignment on special built-in trap is persistent'
a=a
a=b trap - TERM
echo $a
__IN__
b
__OUT__

test_o 'assignment on special built-in unset is persistent'
a=a
a=b unset b
echo $a
__IN__
b
__OUT__

test_O 'function cannot override special built-in colon'
:() { echo not reached; }
:
__IN__

test_O 'function cannot override special built-in dot'
.() { echo not reached; }
. /dev/null
__IN__

test_OE 'function cannot override special built-in break'
break() { echo not reached; }
for i in 1; do
    break
done
__IN__

test_OE 'function cannot override special built-in continue'
continue() { echo not reached; }
for i in 1; do
    continue
done
__IN__

test_OE 'function cannot override special built-in eval'
eval() { echo not reached; }
eval ''
__IN__

test_OE 'function cannot override special built-in exec'
exec() { echo not reached; }
exec
__IN__

test_OE 'function cannot override special built-in exit'
exit() { echo not reached; }
exit
__IN__

test_OE 'function cannot override special built-in export'
export() { echo not reached; }
export a=a
__IN__

test_OE 'function cannot override special built-in readonly'
readonly() { echo not reached; }
readonly a=a
__IN__

test_OE 'function cannot override special built-in return'
return() { echo not reached; }
fn() { return; }
fn
__IN__

test_OE 'function cannot override special built-in set'
set() { echo not reached; }
set ''
__IN__

test_OE 'function cannot override special built-in shift'
shift() { echo not reached; }
shift 0
__IN__

test_E 'function cannot override special built-in times'
times() { echo not reached >&2; }
times
__IN__

test_OE 'function cannot override special built-in trap'
trap() { echo not reached; }
trap - TERM
__IN__

test_OE 'function cannot override special built-in unset'
unset() { echo not reached; }
unset unset
__IN__

# $1 = line no.
# $2 = command name (other than special built-ins)
test_nonspecial_builtin_function_override() {
    testcase "$1" "function overrides non-special command $2" \
        5<&- 3<<__IN__ 4<<__OUT__
$2() { echo function overrides $2; }
$2 XXX
__IN__
function overrides $2
__OUT__
}

test_nonspecial_builtin_function_override "$LINENO" alias
test_nonspecial_builtin_function_override "$LINENO" bg
test_nonspecial_builtin_function_override "$LINENO" cd
test_nonspecial_builtin_function_override "$LINENO" command
test_nonspecial_builtin_function_override "$LINENO" false
test_nonspecial_builtin_function_override "$LINENO" fc
test_nonspecial_builtin_function_override "$LINENO" fg
test_nonspecial_builtin_function_override "$LINENO" getopts
test_nonspecial_builtin_function_override "$LINENO" hash
test_nonspecial_builtin_function_override "$LINENO" jobs
test_nonspecial_builtin_function_override "$LINENO" kill
test_nonspecial_builtin_function_override "$LINENO" pwd
test_nonspecial_builtin_function_override "$LINENO" read
test_nonspecial_builtin_function_override "$LINENO" true
test_nonspecial_builtin_function_override "$LINENO" type
test_nonspecial_builtin_function_override "$LINENO" ulimit
test_nonspecial_builtin_function_override "$LINENO" umask
test_nonspecial_builtin_function_override "$LINENO" unalias
test_nonspecial_builtin_function_override "$LINENO" wait

test_nonspecial_builtin_function_override "$LINENO" grep
test_nonspecial_builtin_function_override "$LINENO" newgrp
test_nonspecial_builtin_function_override "$LINENO" sed

(
setup 'PATH=; unset PATH'

test_OE -e 0 'special built-in colon can be invoked without $PATH'
:
__IN__

test_OE -e 0 'special built-in dot can be invoked without $PATH'
. /dev/null
__IN__

test_OE -e 0 'special built-in break can be invoked without $PATH'
for i in 1; do
    break
done
__IN__

test_OE -e 0 'special built-in continue can be invoked without $PATH'
for i in 1; do
    continue
done
__IN__

test_OE -e 0 'special built-in eval can be invoked without $PATH'
eval ''
__IN__

test_OE -e 0 'special built-in exec can be invoked without $PATH'
exec
__IN__

test_OE -e 0 'special built-in exit can be invoked without $PATH'
exit
__IN__

test_OE -e 0 'special built-in export can be invoked without $PATH'
export a=a
__IN__

test_OE -e 0 'special built-in readonly can be invoked without $PATH'
readonly a=a
__IN__

test_OE -e 0 'special built-in return can be invoked without $PATH'
fn() { return; }
fn
__IN__

test_OE -e 0 'special built-in set can be invoked without $PATH'
set ''
__IN__

test_OE -e 0 'special built-in shift can be invoked without $PATH'
shift 0
__IN__

test_E -e 0 'special built-in times can be invoked without $PATH'
times
__IN__

test_OE -e 0 'special built-in trap can be invoked without $PATH'
trap - TERM
__IN__

test_OE -e 0 'special built-in unset can be invoked without $PATH'
unset unset
__IN__

)

##### Intrinsic built-ins

(
setup 'PATH=; unset PATH'

test_OE -e 0 'intrinsic built-in alias can be invoked without $PATH'
alias a=a
__IN__

# Tested in builtins-y.tst.
#test_OE -e 0 'intrinsic built-in bg can be invoked without $PATH'

test_OE -e 0 'intrinsic built-in cd can be invoked without $PATH'
cd .
__IN__

test_OE -e 0 'intrinsic built-in command can be invoked without $PATH'
command :
__IN__

# Tested in builtins-y.tst.
#test_OE -e 0 'intrinsic built-in fc can be invoked without $PATH'
#test_OE -e 0 'intrinsic built-in fg can be invoked without $PATH'

test_OE -e 0 'intrinsic built-in getopts can be invoked without $PATH'
getopts o o -o
__IN__

test_OE -e 0 'intrinsic built-in hash can be invoked without $PATH'
hash -r
__IN__

test_OE -e 0 'intrinsic built-in jobs can be invoked without $PATH'
jobs
__IN__

test_OE -e 0 'intrinsic built-in kill can be invoked without $PATH'
kill -0 $$
__IN__

test_OE -e 0 'intrinsic built-in read can be invoked without $PATH'
read a
_this_line_is_read_by_the_read_built_in_
__IN__

test_E -e 0 'intrinsic built-in type can be invoked without $PATH'
type type
__IN__

# Tested in builtins-y.tst.
#test_E -e 0 'intrinsic built-in ulimit can be invoked without $PATH'

test_OE -e 0 'intrinsic built-in umask can be invoked without $PATH'
umask 000
__IN__

test_OE -e 0 'intrinsic built-in unalias can be invoked without $PATH'
unalias -a
__IN__

test_OE -e 0 'intrinsic built-in wait can be invoked without $PATH'
wait
__IN__

)

# vim: set ft=sh ts=8 sts=4 sw=4 et:
