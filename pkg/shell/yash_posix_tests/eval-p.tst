# eval-p.tst: test of the eval built-in for any POSIX-compliant shell

posix="true"

test_OE -e 0 'evaluating no operands'
false
eval
__IN__

test_OE -e 0 'evaluating null operands'
false
eval '' '' ''
__IN__

test_oE -e 0 'evaluating some commands'
eval 'echo foo; echo bar'
__IN__
foo
bar
__OUT__

test_oE -e 0 'separator preceding operand'
eval -- 'echo foo'
__IN__
foo
__OUT__

test_oE -e 0 'operands are concatenated with spaces in-between'
eval 'echo foo' 'echo bar'
eval 'echo 1"' '' '"2'
__IN__
foo echo bar
1  2
__OUT__

test_OE -e 23 'exit status of evaluation'
eval '(exit 23)'
__IN__

test_oE -e 0 'effect on environment in evaluation'
a=foo
eval 'a=bar'
echo $a
eval exit
echo not reached
__IN__
bar
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
