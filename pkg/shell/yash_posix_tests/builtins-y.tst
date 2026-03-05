# builtins-y.tst: yash-specific test of built-ins' attributes
../checkfg || skip="true" # %REQUIRETTY%

##### Intrinsic built-ins

if testee -c 'command -bv ulimit' >/dev/null; then
    has_ulimit=true
else
    has_ulimit=false
fi

(
posix="true"
setup 'PATH=; unset PATH'

test_OE -e 0 'intrinsic built-in bg can be invoked without $PATH' -em
"$TESTEE" -c 'kill -STOP $$' && true
bg >/dev/null
wait
__IN__

test_OE -e 0 'intrinsic built-in fg can be invoked without $PATH' -em
:&
fg >/dev/null
__IN__

#TODO: test_OE -e 0 'intrinsic built-in fc can be invoked without $PATH'

(
if ! $has_ulimit; then
    skip="true"
fi

test_E -e 0 'intrinsic built-in ulimit can be invoked without $PATH'
ulimit
__IN__

)

)

# vim: set ft=sh ts=8 sts=4 sw=4 et:
