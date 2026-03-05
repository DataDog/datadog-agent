# eval-y.tst: yash-specific test of the eval built-in

test_OE -e 0 'empty iteration'
false
eval -i
__IN__

test_oE -e 0 'single iteration'
eval -i 'echo foo; echo bar'
__IN__
foo
bar
__OUT__

test_OE -e 0 'single empty iteration'
false
eval -i ''
__IN__

test_oE -e 0 'multiple iteration'
eval -i 'echo foo; echo bar' 'echo 1; echo 2' 'echo a; echo b'
__IN__
foo
bar
1
2
a
b
__OUT__

test_oE -e 0 'iteration with long option'
eval --iteration 'echo foo; echo bar' 'echo 1; echo 2' 'echo a; echo b'
__IN__
foo
bar
1
2
a
b
__OUT__

test_oE -e 13 'exit status in iteration'
(exit 7)
eval -i 'echo a $?; (exit 11)' 'echo b $?; (exit 12)' 'echo c $?; (exit 13)'
__IN__
a 7
b 7
c 7
__OUT__

test_oE -e 0 'effect on environment in iteration'
eval -i 'a=foo'
echo $a
eval -i 'a=${a}bar' 'echo $a' exit 'echo not reached'
__IN__
foo
foobar
__OUT__

test_Oe -e n 'invalid option'
eval --no-such-option
__IN__
eval: `--no-such-option' is not a valid option
__ERR__
#'
#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
