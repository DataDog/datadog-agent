# break-p.tst: test of the break built-in for any POSIX-compliant shell

posix="true"

test_oE 'breaking one for loop, unnested'
for i in 1 2 3; do
    echo in $i
    break 1
    echo out $i
done
echo done $?
__IN__
in 1
done 0
__OUT__

test_oE 'breaking one while loop, unnested'
while true; do
    echo in
    break 1
    echo out
done
echo done $?
__IN__
in
done 0
__OUT__

test_oE 'breaking one until loop, unnested'
until false; do
    echo in
    break 1
    echo out
done
echo done $?
__IN__
in
done 0
__OUT__

test_oE 'breaking one for loop, nested in for loop'
for i in 1 2 3; do
    echo in $i
    for j in a b c; do
        echo in $i $j
        break 1
        echo out $i $j
    done
    echo out $i
done
echo done $?
__IN__
in 1
in 1 a
out 1
in 2
in 2 a
out 2
in 3
in 3 a
out 3
done 0
__OUT__

test_oE 'breaking one for loop, nested in while loop'
i=1
while [ $i -le 3 ]; do
    echo in $i
    for j in a b c; do
        echo in $i $j
        break 1
        echo out $i $j
    done
    echo out $i
    i=$((i+1))
done
echo done $?
__IN__
in 1
in 1 a
out 1
in 2
in 2 a
out 2
in 3
in 3 a
out 3
done 0
__OUT__

test_oE 'breaking one while loop, nested in while loop'
i=1
while [ $i -le 3 ]; do
    echo in outer $i
    while true; do
        echo in inner $i
        break 1
        echo out inner $i
    done
    echo out outer $i
    i=$((i+1))
done
echo done $?
__IN__
in outer 1
in inner 1
out outer 1
in outer 2
in inner 2
out outer 2
in outer 3
in inner 3
out outer 3
done 0
__OUT__

test_oE 'breaking one while loop, nested in until loop'
i=1
until [ $i -gt 3 ]; do
    echo in outer $i
    while true; do
        echo in inner $i
        break 1
        echo out inner $i
    done
    echo out outer $i
    i=$((i+1))
done
echo done $?
__IN__
in outer 1
in inner 1
out outer 1
in outer 2
in inner 2
out outer 2
in outer 3
in inner 3
out outer 3
done 0
__OUT__

test_oE 'breaking one until loop, nested in until loop'
i=1
until [ $i -gt 3 ]; do
    echo in outer $i
    until false; do
        echo in inner $i
        break 1
        echo out inner $i
    done
    echo out outer $i
    i=$((i+1))
done
echo done $?
__IN__
in outer 1
in inner 1
out outer 1
in outer 2
in inner 2
out outer 2
in outer 3
in inner 3
out outer 3
done 0
__OUT__

test_oE 'breaking one until loop in function, nested in until loop'
func() {
    until false; do
        echo in inner $i
        break 1
        echo out inner $i
    done
    echo out func
}
i=1
until [ $i -gt 2 ]; do
    echo in outer $i
    func
    echo out outer $i
    i=$((i+1))
done
__IN__
in outer 1
in inner 1
out func
out outer 1
in outer 2
in inner 2
out func
out outer 2
__OUT__

test_oE 'breaking two for loops, outermost'
for i in 1 2 3; do
    echo in $i
    for j in a b c; do
        echo in $i $j
        break 2
        echo out $i $j
    done
    echo out $i
done
echo done $?
__IN__
in 1
in 1 a
done 0
__OUT__

test_oE 'breaking for and while loops, outermost'
i=1
while [ $i -le 3 ]; do
    echo in $i
    for j in a b c; do
        echo in $i $j
        break 2
        echo out $i $j
    done
    echo out $i
    i=$((i+1))
done
echo done $?
__IN__
in 1
in 1 a
done 0
__OUT__

test_oE 'breaking two while loops, outermost'
i=1
while [ $i -le 3 ]; do
    echo in outer $i
    while true; do
        echo in inner $i
        break 2
        echo out inner $i
    done
    echo out outer $i
    i=$((i+1))
done
echo done $?
__IN__
in outer 1
in inner 1
done 0
__OUT__

test_oE 'breaking while and until loops, outermost'
i=1
until [ $i -gt 3 ]; do
    echo in outer $i
    while true; do
        echo in inner $i
        break 2
        echo out inner $i
    done
    echo out outer $i
    i=$((i+1))
done
echo done $?
__IN__
in outer 1
in inner 1
done 0
__OUT__

test_oE 'breaking two for loops, nested in another for loop'
for i in 1 2 3; do
    echo in $i
    for j in a b c; do
        echo in $i $j
        for k in + -; do
            echo in $i $j $k
            break 2
            echo out $i $j $k
        done
        echo out $i $j
    done
    echo out $i
done
echo done $?
__IN__
in 1
in 1 a
in 1 a +
out 1
in 2
in 2 a
in 2 a +
out 2
in 3
in 3 a
in 3 a +
out 3
done 0
__OUT__

test_oE 'breaking three for loops, outermost'
for i in 1 2 3; do
    echo in $i
    for j in a b c; do
        echo in $i $j
        for k in + -; do
            echo in $i $j $k
            break 3
            echo out $i $j $k
        done
        echo out $i $j
    done
    echo out $i
done
echo done $?
__IN__
in 1
in 1 a
in 1 a +
done 0
__OUT__

test_oE 'default operand is 1'
for i in 1; do
    echo in $i
    for j in a; do
        echo in $i $j
        for k in +; do
            echo in $i $j $k
            break
            echo out $i $j $k
        done
        echo out $i $j
    done
    echo out $i
done
echo done $?
__IN__
in 1
in 1 a
in 1 a +
out 1 a
out 1
done 0
__OUT__

test_OE -e 0 'exit status of break with $? > 0'
for i in 1; do
    false
    break
done
__IN__

test_O -d -e n 'zero operand'
for i in 1; do
    break 0
done
__IN__

test_OE 'breaking one more than actual nest level one'
for i in 1; do
    break 2
    echo not reached
done
__IN__

test_OE 'breaking one more than actual nest level two'
for i in 1; do
    for j in a; do
        break 3
        echo not reached 1
    done
    echo not reached 2
done
__IN__

test_OE 'breaking much more than actual nest level one'
for i in 1; do
    break 100
    echo not reached
done
__IN__

# This is a questionable case. Is this really a "lexically enclosing" loop as
# defined in POSIX? Most shells do support this case.
test_OE 'breaking out of eval'
for i in 1; do
    eval break
    echo not reached
done
__IN__

test_OE 'breaking with !'
for i in 1; do
    ! break
    echo not reached
done
__IN__

test_OE 'breaking before &&'
for i in 1; do
    break && echo not reached 1
    echo not reached 2 $?
done
__IN__

test_OE 'breaking after &&'
for i in 1; do
    true && break
    echo not reached $?
done
__IN__

test_OE 'breaking before ||'
for i in 1; do
    break || echo not reached 1
    echo not reached 2 $?
done
__IN__

test_OE 'breaking after ||'
for i in 1; do
    false || break
    echo not reached $?
done
__IN__

test_OE 'breaking out of brace'
for i in 1; do
    { break; }
    echo not reached
done
__IN__

test_OE 'breaking out of if'
for i in 1; do
    if break; then echo not reached then; else echo not reached else; fi
    echo not reached
done
__IN__

test_OE 'breaking out of then'
for i in 1; do
    if true; then break; echo not reached then; else not reached else; fi
    echo not reached
done
__IN__

test_OE 'breaking out of else'
for i in 1; do
    if false; then echo not reached then; else break; not reached else; fi
    echo not reached
done
__IN__

test_OE 'breaking out of case'
for i in 1; do
    case x in
        x)
            break
            echo not reached in case
    esac
    echo not reached after esac
done
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
