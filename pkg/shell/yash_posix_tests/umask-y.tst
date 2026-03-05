# umask-y.tst: yash-specific test of the umask built-in

# $1 = $LINENO, $2 = umask
test_print_non_symbolic() (
    umask "$2"
    testcase "$1" -e 0 \
        "without operand, current umask is printed, non-symbolic, $2" \
        3<<\__IN__ 4<<__OUT__ 5</dev/null
umask
__IN__
$2
__OUT__
)

test_print_non_symbolic "$LINENO" 0000
test_print_non_symbolic "$LINENO" 0001
test_print_non_symbolic "$LINENO" 0002
test_print_non_symbolic "$LINENO" 0004
test_print_non_symbolic "$LINENO" 0010
test_print_non_symbolic "$LINENO" 0020
test_print_non_symbolic "$LINENO" 0040
test_print_non_symbolic "$LINENO" 0100
test_print_non_symbolic "$LINENO" 0200
test_print_non_symbolic "$LINENO" 0400
test_print_non_symbolic "$LINENO" 0356
test_print_non_symbolic "$LINENO" 0017

# $1 = $LINENO, $2 = umask, $3 = expected output
test_print_symbolic() (
    umask "$2"
    testcase "$1" -e 0 \
        "without operand, current umask is printed, symbolic, $2" \
        3<<\__IN__ 4<<__OUT__ 5</dev/null
umask -S
umask --symbolic
__IN__
$3
$3
__OUT__
)

test_print_symbolic "$LINENO" 0777 u=,g=,o=
test_print_symbolic "$LINENO" 0624 u=x,g=rx,o=wx
test_print_symbolic "$LINENO" 0153 u=rw,g=w,o=r
test_print_symbolic "$LINENO" 0000 u=rwx,g=rwx,o=rwx

test_Oe -e 2 'too many operands'
umask a+r a-w
__IN__
umask: too many operands are specified
__ERR__

test_Oe -e 2 'invalid option'
umask --no-such-option
__IN__
umask: `--no-such-option' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'invalid operand (numeric)'
umask 09
__IN__
umask: `09' is not a valid mask specification
__ERR__
#'
#`

test_Oe -e 2 'invalid operand (symbolic)'
umask 'a*rwx'
__IN__
umask: `a*rwx' is not a valid mask specification
__ERR__
#'
#`

test_O -d -e 1 'printing to closed stream'
umask >&-
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
