# pwd-y.tst: yash-specific test of the pwd built-in

test_Oe -e 2 'invalid option'
pwd --no-such-option
__IN__
pwd: `--no-such-option' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'invalid operand'
pwd unexpected_operand
__IN__
pwd: no operand is expected
__ERR__

test_O -d -e 1 'printing to closed stream'
pwd >&-
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
