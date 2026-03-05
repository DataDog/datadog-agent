# ulimit-y.tst: yash-specific test of the ulimit built-in

if ! testee -c 'command -bv ulimit' >/dev/null; then
    skip="true"
fi

test_Oe -e 2 'too many operands (w/o -a)'
ulimit 0 0
__IN__
ulimit: too many operands are specified
__ERR__

test_Oe -e 2 'too many operands (with -a)'
ulimit -a 0
__IN__
ulimit: no operand is expected
__ERR__

test_Oe -e 2 'invalid option --xxx'
ulimit --no-such=option
__IN__
ulimit: `--no-such=option' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'specifying -a and -f at once'
ulimit -a -f
__IN__
ulimit: the -a option cannot be used with the -f option
__ERR__

test_Oe -e 2 'invalid operand (non-numeric)'
ulimit X
__IN__
ulimit: `X' is not a valid integer
__ERR__
#'
#`

test_Oe -e 2 'invalid operand (non-integral)'
ulimit 1.0
__IN__
ulimit: `1.0' is not a valid integer
__ERR__
#'
#`

test_Oe -e 2 'invalid operand (negative)'
ulimit -- -1
__IN__
ulimit: `-1' is not a valid integer
__ERR__
#'
#`

test_O -d -e 1 'printing to closed output stream'
ulimit >&-
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
