# continue-y.tst: yash-specific test of the continue built-in

echo 'continue; echo \$?=$?' >continue

test_oe 'continuing out of dot'
for i in 1 2; do
    echo $i
    . ./continue
done
__IN__
1
$?=2
2
$?=2
__OUT__
continue: not in a loop
continue: not in a loop
__ERR__

test_oe 'continuing out of function'
c() { continue; echo \$?=$?; }
for i in 1 2; do
    echo $i
    c
done
__IN__
1
$?=2
2
$?=2
__OUT__
continue: not in a loop
continue: not in a loop
__ERR__

test_oe 'continuing out of subshell'
for i in 1; do
    (continue) || echo ok
done
__IN__
ok
__OUT__
continue: not in a loop
__ERR__

test_oe 'continuing out of trap'
trap 'continue || echo trapped' USR1
for i in 1; do
    kill -USR1 $$
    echo ok
done
__IN__
trapped
ok
__OUT__
continue: not in a loop
__ERR__

test_oE 'continuing iteration, unnested, short option'
eval -i 'echo 1' 'continue -i; echo not reached' 'echo continued'
__IN__
1
continued
__OUT__

test_oE 'continuing iteration, unnested, long option'
eval -i 'echo 1' 'continue --iteration; echo not reached' 'echo continued'
__IN__
1
continued
__OUT__

test_OE -e 17 'exit status of continued iteration'
eval -i '(exit 17); continue -i'
__IN__

test_oE 'continuing nested iteration'
eval -i 'eval -i "continue -i; echo not reached" "echo 1"; echo 2'
__IN__
1
2
__OUT__

test_OE 'continuing loop out of iteration'
for i in 1; do
    eval -i continue 'echo not reached 1'
    echo not reached 2
done
__IN__

test_o 'continuing loop out of auxiliary not allowed'
COMMAND_NOT_FOUND_HANDLER=(continue 'echo reached 1 $?')
for i in 1; do
    ./_no_such_command_
    echo reached 2 $?
done
__IN__
reached 1 0
reached 2 127
__OUT__

test_oE 'continuing iteration out of eval'
eval -i 'eval "continue -i"; echo not reached' 'echo continued'
__IN__
continued
__OUT__

echo 'continue -i' >continue-i

test_oE 'continuing iteration out of dot'
eval -i '. ./continue-i; echo not reached' 'echo continued'
__IN__
continued
__OUT__

test_oE 'continuing iteration out of loop'
eval -i 'for i in 1; do continue -i; done; echo not reached' 'echo continued'
__IN__
continued
__OUT__

test_Oe -e n 'continuing without target loop'
continue
__IN__
continue: not in a loop
__ERR__

test_Oe -e n 'continuing without target iteration'
continue -i
__IN__
continue: not in an iteration
__ERR__

test_Oe -e n 'too many operands'
continue 1 2
__IN__
continue: too many operands are specified
__ERR__

test_Oe -e n 'operand and -i'
continue -i 1
__IN__
continue: no operand is expected
__ERR__

test_Oe -e n 'invalid option'
continue --no-such-option
__IN__
continue: `--no-such-option' is not a valid option
__ERR__
#'
#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
