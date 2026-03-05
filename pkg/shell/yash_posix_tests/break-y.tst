# break-y.tst: yash-specific test of the break built-in

echo 'break; echo \$?=$?' >break

test_oe 'breaking out of dot'
for i in 1 2; do
    echo $i
    . ./break
done
__IN__
1
$?=2
2
$?=2
__OUT__
break: not in a loop
break: not in a loop
__ERR__

test_oe 'breaking out of function'
b() { break; echo \$?=$?; }
for i in 1 2; do
    echo $i
    b
done
__IN__
1
$?=2
2
$?=2
__OUT__
break: not in a loop
break: not in a loop
__ERR__

test_oe 'breaking out of subshell'
for i in 1; do
    (break) || echo ok
done
__IN__
ok
__OUT__
break: not in a loop
__ERR__

test_oe 'breaking out of trap'
trap 'break || echo trapped' USR1
for i in 1; do
    kill -USR1 $$
    echo ok
done
__IN__
trapped
ok
__OUT__
break: not in a loop
__ERR__

test_oE 'breaking iteration, unnested, short option'
eval -i 'echo 1' \
    '(exit 13); break -i; echo not reached 1' \
    'echo not reached 2'
echo $?
__IN__
1
13
__OUT__

test_oE 'breaking iteration, unnested, long option'
eval -i 'echo 1' \
    '(exit 13); break --iteration; echo not reached 1' \
    'echo not reached 2'
echo $?
__IN__
1
13
__OUT__

test_oE 'breaking nested iteration'
eval -i 'eval -i "break -i" "echo not reached"; echo broke'
__IN__
broke
__OUT__

test_OE 'breaking loop out of iteration'
for i in 1; do
    eval -i break 'echo not reached 1'
    echo not reached 2
done
__IN__

test_o 'breaking loop out of auxiliary not allowed'
COMMAND_NOT_FOUND_HANDLER=(break 'echo reached 1 $?')
for i in 1; do
    ./_no_such_command_
    echo reached 2 $?
done
__IN__
reached 1 0
reached 2 127
__OUT__

test_OE 'breaking iteration out of eval'
eval -i 'eval "break -i"; echo not reached 1' 'echo not reached 2'
__IN__

echo 'break -i' >break-i

test_OE 'breaking iteration out of dot'
eval -i '. ./break-i; echo not reached 1' 'echo not reached 2'
__IN__

test_OE 'breaking iteration out of loop'
eval -i 'for i in 1; do break -i; done; echo not reached 1' \
    'echo not reached 2'
__IN__

test_Oe -e n 'breaking without target loop'
break
__IN__
break: not in a loop
__ERR__

test_Oe -e n 'breaking without target iteration'
break -i
__IN__
break: not in an iteration
__ERR__

test_Oe -e n 'too many operands'
break 1 2
__IN__
break: too many operands are specified
__ERR__

test_Oe -e n 'operand and -i'
break -i 1
__IN__
break: no operand is expected
__ERR__

test_Oe -e n 'invalid option'
break --no-such-option
__IN__
break: `--no-such-option' is not a valid option
__ERR__
#'
#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
