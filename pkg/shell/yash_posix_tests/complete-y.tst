# complete-y.tst: yash-specific test of the complete built-in

if ! testee -c 'command -bv complete' >/dev/null; then
    skip="true"
fi

test_oE -e 0 'complete is an elective built-in'
command -V complete
__IN__
complete: an elective built-in
__OUT__

test_Oe -e 2 'invalid option'
complete --no-such-option
__IN__
complete: `--no-such-option' is not a valid option
__ERR__
#`

test_Oe -e 2 'not during completion'
complete
__IN__
complete: the complete built-in can be used during command line completion only
__ERR__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
