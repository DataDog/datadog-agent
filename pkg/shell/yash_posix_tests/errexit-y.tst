# errexit-y.tst: yash-specific test of the errexit option

# I think the shell should exit for all cases below, but POSIX and existing
# implementations vary...

# An expansion error in a non-interactive shell causes immediate exit of the
# shell (regardless of errexit), so expansion errors should be tested in an
# interactive shell.

setup 'set -e'

test_O -e n 'expansion error in case word' -i +m
case ${a?} in (*) esac
echo not reached
__IN__

test_O -e n 'expansion error in case pattern' -i +m
case a in (${a?}) esac
echo not reached
__IN__

test_O -e n 'expansion error in for word' -i +m
for i in ${a?}; do echo not reached; done
echo not reached
__IN__

test_O -e n 'assignment error in for word' -i +m
readonly i=X
for i in 1; do echo not reached; done
echo not reached
__IN__

test_O -e n 'redirection error on subshell'
( :; ) <_no_such_file_
echo not reached
__IN__

test_O -e n 'redirection error on grouping'
{ :; } <_no_such_file_
echo not reached
__IN__

test_O -e n 'redirection error on for loop'
for i in i; do :; done <_no_such_file_
echo not reached
__IN__

test_O -e n 'redirection error on case'
case i in esac <_no_such_file_
echo not reached
__IN__

test_O -e n 'redirection error on if'
if :; then :; fi <_no_such_file_
echo not reached
__IN__

test_O -e n 'redirection error on while loop'
while echo not reached; false; do :; done <_no_such_file_
echo not reached
__IN__

test_O -e n 'redirection error on until loop'
until echo not reached; do :; done <_no_such_file_
echo not reached
__IN__

(
if ! testee -c 'command -v [[' >/dev/null; then
    skip="true"
fi

test_O -e 2 'expansion error in double-bracket command' -i +m
[[ ${a?} ]]
echo not reached
__IN__

test_O -e 2 'redirection error on double-bracket command'
exec 3>&1
[[ $(echo not reached >&3) ]] <_no_such_file_
echo not reached
__IN__

)

# vim: set ft=sh ts=8 sts=4 sw=4 et:
