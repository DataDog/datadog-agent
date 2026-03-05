# export-p.tst: test of the export built-in for any POSIX-compliant shell

posix="true"

test_oE -e 0 'exporting one variable' -e
export a=bar
echo 1 $a
sh -c 'echo 2 $a'
__IN__
1 bar
2 bar
__OUT__

test_oE -e 0 'exporting many variables' -e
a=X b=B c=X
export a=A b c=C
echo 1 $a $b $c
sh -c 'echo 2 $a $b $c'
__IN__
1 A B C
2 A B C
__OUT__

test_oE -e 0 'separator preceding operand' -e
export -- a=foo
echo 1 $a
sh -c 'echo 2 $a'
__IN__
1 foo
2 foo
__OUT__

test_oE -e 0 'reusing printed exported variables'
export a=A
e="$(export -p)"
unset a
a=X
eval "$e"
sh -c 'echo $a'
__IN__
A
__OUT__

test_oE 'exporting with assignments'
a=A export b=B
# POSIX requires $a to persist after the export built-in,
# but it is unspecified whether $a is exported.
echo $a
# $a does not affect $b being exported.
sh -c 'echo $b'
__IN__
A
B
__OUT__

test_O -d -e n 'read-only variable cannot be re-assigned'
readonly a=1
export a=2
# The export built-in fails because of the readonly variable.
# Since it is a special built-in, the non-interactive shell exits.
echo not reached
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
