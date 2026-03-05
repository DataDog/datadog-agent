# pipeline-p.tst: test of pipeline for any POSIX-compliant shell

posix="true"

test_o '2-command pipeline'
echo foo | cat
__IN__
foo
__OUT__

test_o '3-command pipeline'
printf '%s\n' foo bar | tail -n 1 | cat
__IN__
bar
__OUT__

test_o 'linebreak after |'
printf '%s\n' foo bar |
tail -n 1 | 
    
cat
__IN__
bar
__OUT__

test_oE 'without pipefail, exit status of pipeline is from last command'
exit 0 | exit 0 | exit 0
echo a $?
exit 1 | exit 2 | exit 0
echo b $?
exit 3 | exit 0 | exit 0
echo c $?
exit 0 | exit 0 | exit 4
echo d $?
exit 5 | exit 6 | exit 7
echo e $?
__IN__
a 0
b 0
c 0
d 4
e 7
__OUT__

test_oE 'exit status of negated pipelines (without pipefail)'
! exit 0 | exit 0 | exit 0
echo a $?
! exit 1 | exit 2 | exit 0
echo b $?
! exit 3 | exit 0 | exit 0
echo c $?
! exit 0 | exit 0 | exit 4
echo d $?
! exit 5 | exit 6 | exit 7
echo e $?
__IN__
a 1
b 1
c 1
d 0
e 0
__OUT__

test_oE 'with pipefail, last failed command determines exit status' -o pipefail
exit 0 | exit 0 | exit 0
echo a $?
exit 1 | exit 2 | exit 0
echo b $?
exit 3 | exit 0 | exit 0
echo c $?
exit 0 | exit 0 | exit 4
echo d $?
exit 5 | exit 6 | exit 7
echo e $?
__IN__
a 0
b 2
c 3
d 4
e 7
__OUT__

test_oE 'exit status of negated pipelines (with pipefail)' -o pipefail
! exit 0 | exit 0 | exit 0
echo a $?
! exit 1 | exit 2 | exit 0
echo b $?
! exit 3 | exit 0 | exit 0
echo c $?
! exit 0 | exit 0 | exit 4
echo d $?
! exit 5 | exit 6 | exit 7
echo e $?
__IN__
a 1
b 0
c 0
d 0
e 0
__OUT__

test_OE -e 0 'pipeline enabling pipefail does not affect itself'
false | set -o pipefail
__IN__

test_oE 'stdin for first command & stdout for last are not modified'
cat | tail -n 1
foo
bar
__IN__
bar
__OUT__

test_Oe 'stderr is not modified'
(echo >&2) | (echo >&2)
__IN__


__ERR__

test_oE 'compound commands in pipeline'
{
    echo foo
    echo bar
} | (
    tail -n 1
    echo baz
) | if true; then
    cat
else
    :
fi
__IN__
bar
baz
__OUT__

test_OE 'redirection overrides pipeline'
echo foo >/dev/null | cat
echo foo | </dev/null cat
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
