# lineno-p.tst: test of the LINENO variable for any POSIX-compliant shell

posix="true"

# POSIX requires the $LINENO variable to be effective only in a script or
# function. However, the meaning of "script" is vague and existing shells
# disagree as to how line numbers are counted within functions. We only test
# common behavior of the shells in this file. More details are tested in
# lineno-y.tst as yash-specific behavior.

test_oE -e 0 'LINENO starts from 1' -s
echo $LINENO
__IN__
1
__OUT__

test_oE -e 0 'LINENO increments for each line' -s
echo $LINENO
echo $LINENO

echo $LINENO
__IN__
1
2
4
__OUT__

test_oE -e 0 'LINENO after expansions' -s
: $(echo foo
echo \
baz)
echo $LINENO
: ${foo#
bar \
baz}
echo $LINENO
: $((1
+ \
2))
echo $LINENO
__IN__
4
8
12
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
