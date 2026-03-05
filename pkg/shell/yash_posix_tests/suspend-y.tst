# suspend-y.tst: yash-specific test of the suspend built-in

test_oE -e 0 'suspend is an elective built-in'
command -V suspend
__IN__
suspend: an elective built-in
__OUT__

test_Oe -e 2 'too many operands'
suspend foo
__IN__
suspend: no operand is expected
__ERR__

test_Oe -e 2 'invalid option --xxx'
suspend --no-such=option
__IN__
suspend: `--no-such=option' is not a valid option
__ERR__
#'
#`

test_O -d -e 127 'suspend built-in is unavailable in POSIX mode' --posix
echo echo not reached > suspend
chmod a+x suspend
PATH=$PWD:$PATH
suspend --help
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
