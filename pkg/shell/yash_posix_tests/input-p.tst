# input-p.tst: test of input processing for any POSIX-compliant shell

posix="true"

# Note that this test case depends on the fact that run-test.sh passes the
# input using a regular file. The test would fail if the input was not
# seekable. See also the "Input files" section in POSIX.1-2008, 1.4 Utility
# Description Defaults.
test_oE 'no input more than needed is read'
"$TESTEE" -c 'read -r line && printf "%s\n" "$line"'
echo - this line is consumed by read and printed by printf
echo - this line is consumed and executed by shell
__IN__
echo - this line is consumed by read and printed by printf
- this line is consumed and executed by shell
__OUT__

test_x -e 0 'exit status of empty input'
__IN__

test_x -e 0 'exit status of input containing blank lines only'


__IN__

test_x -e 0 'exit status of input containing blank lines and comments only'

# foo

# bar
__IN__

test_oE 'long line'
echo 1                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        2
__IN__
1 2
__OUT__

test_oE 'line continuation and long line'
echo \
1                                                                             \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              \
                                                                              2
__IN__
1 2
__OUT__

printf 'alias false=:\nfalse\n' >inputfile.sh

test_x -e 0 'shell input is line-wise (file)' ./inputfile.sh
__IN__

test_x -e 0 'shell input is line-wise (standard input)'
alias false=:
false
__IN__

test_x -e 0 'shell input is line-wise (-c)' -c 'alias false=:
false'
__IN__

test_x -e 0 'shell input is line-wise (command substitution)'
x=$(alias false=:
false)
__IN__

test_x -e 0 'shell input is line-wise (eval)'
eval 'alias false=:
false'
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
