# exit-y.tst: yash-specific test of the exit built-in

test_Oe -e 2 'too many operands'
exit 1 2
__IN__
exit: too many operands are specified
__ERR__

test_Oe -e 2 'invalid operand: not a integer'
exit x
echo not reached
__IN__
exit: `x' is not a valid integer
__ERR__
#'
#`

test_Oe -e 2 'invalid operand: negative integer'
exit -- -100
echo not reached
__IN__
exit: `-100' is not a valid integer
__ERR__
#'
#`

test_Oe -e 2 'invalid option'
exit --no-such-option
__IN__
exit: `--no-such-option' is not a valid option
__ERR__
#'
#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
