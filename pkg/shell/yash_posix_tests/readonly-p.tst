# readonly-p.tst: test of the readonly built-in for any POSIX-compliant shell

posix="true"

test_o -d -e n 'making one variable read-only'
readonly a=bar
echo $a
a=X # This should fail, and the shell should exit.
echo not reached
__IN__
bar
__OUT__

test_o -d 'making many variables read-only'
a=X b=B c=X
readonly a=A b c=C
echo $a $b $c
(
    a=X # This should fail, and the subshell should exit.
    echo not reached
) || (
    b=Y # This should fail, and the subshell should exit.
    echo not reached
) || (
    c=Z # This should fail, and the subshell should exit.
    echo not reached
) ||
echo $a $b $c # This should print the values passed to the readonly built-in.
__IN__
A B C
A B C
__OUT__

test_oE -e 0 'separator preceding operand' -e
readonly -- a=foo
echo $a
__IN__
foo
__OUT__

# This test is in readonly-y.tst because it fails on some existing shells
# because of pre-defined read-only variables.
#test_x 'reusing printed read-only variables'

test_O -d -e n 'read-only variable cannot be re-assigned'
readonly a=1
readonly a=2
# The readonly built-in fails because of the readonly variable.
# Since it is a special built-in, the non-interactive shell exits.
echo not reached
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
