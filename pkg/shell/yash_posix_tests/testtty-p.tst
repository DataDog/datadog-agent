# testtty-p.tst: test of the test built-in for any POSIX-compliant shell
../checkfg || skip="true" # %REQUIRETTY%

if ! testee -c 'command -bv test' >/dev/null; then
    skip="true"
fi

posix="true"

test_OE -e 1 'unary -t: empty operand'
test -t ''
__IN__

test_OE -e 1 'unary -t: non-numeric operand'
test -t x
__IN__

test_OE -e 1 'unary -t: negative operand'
test -t -10
__IN__

test_OE -e 1 'unary -t: closed file descriptor 0'
test -t 0 0>&-
__IN__

test_OE -e 1 'unary -t: non-tty file descriptor 0'
test -t 0 0</dev/null
__IN__

test_OE -e 0 'unary -t: tty file descriptor 0'
test -t 0 0<>/dev/tty
__IN__

test_OE -e 1 'unary -t: closed file descriptor 5'
test -t 5 5>&-
__IN__

test_OE -e 1 'unary -t: non-tty file descriptor 5'
test -t 5 5</dev/null
__IN__

test_OE -e 0 'unary -t: tty file descriptor 5'
test -t 5 5<>/dev/tty
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
