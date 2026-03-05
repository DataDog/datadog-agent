# times-y.tst: yash-specific test of the times built-in

test_Oe -e 2 'too many operands'
times foo
__IN__
times: no operand is expected
__ERR__

test_Oe -e 2 'invalid option --xxx'
times --no-such=option
__IN__
times: `--no-such=option' is not a valid option
__ERR__
#'
#`

test_O -d -e 1 'printing to closed output stream'
times >&-
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
