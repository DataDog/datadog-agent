# continue-p.tst: test of the continue built-in for any POSIX-compliant shell

posix="true"

test_oE 'continuing one for loop, unnested'
for i in 1 2 3; do
    echo in $i
    continue 1
    echo out $i
done
echo done $?
__IN__
in 1
in 2
in 3
done 0
__OUT__

test_oE 'continuing one while loop, unnested'
i=1
while [ $i -le 3 ]; do
    echo in $i
    i=$((i+1))
    continue 1
    echo out $i
done
echo done $?
__IN__
in 1
in 2
in 3
done 0
__OUT__

test_oE 'continuing one until loop, unnested'
i=1
until [ $i -gt 3 ]; do
    echo in $i
    i=$((i+1))
    continue 1
    echo out $i
done
echo done $?
__IN__
in 1
in 2
in 3
done 0
__OUT__

test_oE 'continuing one for loop, nested in for loop'
for i in 1 2 3; do
    echo in $i
    for j in a b c; do
        echo in $i $j
        continue 1
        echo out $i $j
    done
    echo out $i
done
echo done $?
__IN__
in 1
in 1 a
in 1 b
in 1 c
out 1
in 2
in 2 a
in 2 b
in 2 c
out 2
in 3
in 3 a
in 3 b
in 3 c
out 3
done 0
__OUT__

test_oE 'continuing one for loop, nested in while loop'
i=1
while [ $i -le 3 ]; do
    echo in $i
    for j in a b c; do
        echo in $i $j
        continue 1
        echo out $i $j
    done
    echo out $i
    i=$((i+1))
done
echo done $?
__IN__
in 1
in 1 a
in 1 b
in 1 c
out 1
in 2
in 2 a
in 2 b
in 2 c
out 2
in 3
in 3 a
in 3 b
in 3 c
out 3
done 0
__OUT__

test_oE 'continuing one while loop, nested in while loop'
i=1
while [ $i -le 3 ]; do
    echo in outer $i
    j=1
    while [ $j -le 3 ]; do
        echo in inner $i $j
        j=$((j+1))
        continue 1
        echo out inner $i $j
    done
    echo out outer $i
    i=$((i+1))
done
echo done $?
__IN__
in outer 1
in inner 1 1
in inner 1 2
in inner 1 3
out outer 1
in outer 2
in inner 2 1
in inner 2 2
in inner 2 3
out outer 2
in outer 3
in inner 3 1
in inner 3 2
in inner 3 3
out outer 3
done 0
__OUT__

test_oE 'continuing one while loop, nested in until loop'
i=1
until [ $i -gt 3 ]; do
    echo in outer $i
    j=1
    while [ $j -le 3 ]; do
        echo in inner $i $j
        j=$((j+1))
        continue 1
        echo out inner $i $j
    done
    echo out outer $i
    i=$((i+1))
done
echo done $?
__IN__
in outer 1
in inner 1 1
in inner 1 2
in inner 1 3
out outer 1
in outer 2
in inner 2 1
in inner 2 2
in inner 2 3
out outer 2
in outer 3
in inner 3 1
in inner 3 2
in inner 3 3
out outer 3
done 0
__OUT__

test_oE 'continuing one until loop, nested in until loop'
i=1
until [ $i -gt 3 ]; do
    echo in outer $i
    j=1
    until [ $j -gt 3 ]; do
        echo in inner $i $j
        j=$((j+1))
        continue 1
        echo out inner $i $j
    done
    echo out outer $i
    i=$((i+1))
done
echo done $?
__IN__
in outer 1
in inner 1 1
in inner 1 2
in inner 1 3
out outer 1
in outer 2
in inner 2 1
in inner 2 2
in inner 2 3
out outer 2
in outer 3
in inner 3 1
in inner 3 2
in inner 3 3
out outer 3
done 0
__OUT__

test_oE 'continuing two for loops, outermost'
for i in 1 2 3; do
    echo in $i
    for j in a b c; do
        echo in $i $j
        continue 2
        echo out $i $j
    done
    echo out $i
done
echo done $?
__IN__
in 1
in 1 a
in 2
in 2 a
in 3
in 3 a
done 0
__OUT__

test_oE 'continuing for and while loops, outermost'
i=1
while [ $i -le 3 ]; do
    echo in $i
    for j in a b c; do
        echo in $i $j
        i=$((i+1))
        continue 2
        echo out $i $j
    done
    echo out $i
done
echo done $?
__IN__
in 1
in 1 a
in 2
in 2 a
in 3
in 3 a
done 0
__OUT__

test_oE 'continuing two while loops, outermost'
i=1
while [ $i -le 3 ]; do
    echo in outer $i
    while true; do
        echo in inner $i
        i=$((i+1))
        continue 2
        echo out inner $i
    done
    echo out outer $i
done
echo done $?
__IN__
in outer 1
in inner 1
in outer 2
in inner 2
in outer 3
in inner 3
done 0
__OUT__

test_oE 'continuing while and until loops, outermost'
i=1
until [ $i -gt 3 ]; do
    echo in outer $i
    while true; do
        echo in inner $i
        i=$((i+1))
        continue 2
        echo out inner $i
    done
    echo out outer $i
done
echo done $?
__IN__
in outer 1
in inner 1
in outer 2
in inner 2
in outer 3
in inner 3
done 0
__OUT__

test_oE 'continuing two for loops, nested in another for loop'
for i in 1 2 3; do
    echo in $i
    for j in a b c; do
        echo in $i $j
        for k in + -; do
            echo in $i $j $k
            continue 2
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
in 1 b
in 1 b +
in 1 c
in 1 c +
out 1
in 2
in 2 a
in 2 a +
in 2 b
in 2 b +
in 2 c
in 2 c +
out 2
in 3
in 3 a
in 3 a +
in 3 b
in 3 b +
in 3 c
in 3 c +
out 3
done 0
__OUT__

test_oE 'continuing three for loops, outermost'
for i in 1 2 3; do
    echo in $i
    for j in a b c; do
        echo in $i $j
        for k in + -; do
            echo in $i $j $k
            continue 3
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
in 2
in 2 a
in 2 a +
in 3
in 3 a
in 3 a +
done 0
__OUT__

test_oE 'default operand is 1'
for i in 1; do
    echo in $i
    for j in a; do
        echo in $i $j
        for k in + -; do
            echo in $i $j $k
            continue
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
in 1 a -
out 1 a
out 1
done 0
__OUT__

test_OE -e 0 'exit status of continue with $? > 0'
for i in 1; do
    false
    continue
done
__IN__

test_O -d -e n 'zero operand'
for i in 1; do
    continue 0
done
__IN__

test_OE 'continuing one more than actual nest level one'
for i in 1; do
    continue 2
    echo not reached
done
__IN__

test_OE 'continuing one more than actual nest level two'
for i in 1; do
    for j in a; do
        continue 3
        echo not reached 1
    done
    echo not reached 2
done
__IN__

test_OE 'continuing much more than actual nest level one'
for i in 1; do
    continue 100
    echo not reached
done
__IN__

# This is a questionable case. Is this really a "lexically enclosing" loop as
# defined in POSIX? Most shells do support this case.
test_oE 'continuing out of eval'
for i in 1 2; do
    echo $i
    eval continue
    echo not reached
done
__IN__
1
2
__OUT__

test_OE 'continuing with !'
for i in 1; do
    ! continue
    echo not reached
done
__IN__

test_OE 'continuing before &&'
for i in 1; do
    continue && echo not reached 1
    echo not reached 2 $?
done
__IN__

test_OE 'continuing after &&'
for i in 1; do
    true && continue
    echo not reached $?
done
__IN__

test_OE 'continuing before ||'
for i in 1; do
    continue || echo not reached 1
    echo not reached 2 $?
done
__IN__

test_OE 'continuing after ||'
for i in 1; do
    false || continue
    echo not reached $?
done
__IN__

test_OE 'continuing out of brace'
for i in 1; do
    { continue; }
    echo not reached
done
__IN__

test_OE 'continuing out of if'
for i in 1; do
    if continue; then echo not reached then; else echo not reached else; fi
    echo not reached
done
__IN__

test_OE 'continuing out of then'
for i in 1; do
    if true; then continue; echo not reached then; else not reached else; fi
    echo not reached
done
__IN__

test_OE 'continuing out of else'
for i in 1; do
    if false; then echo not reached then; else continue; not reached else; fi
    echo not reached
done
__IN__

test_OE 'continuing out of case'
for i in 1; do
    case x in
        x)
            continue
            echo not reached in case
    esac
    echo not reached after esac
done
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
