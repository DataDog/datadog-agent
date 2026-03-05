# errexit-p.tst: test of the errexit option for any POSIX-compliant shell

posix="true"

test_o -e 0 'noerrexit: successful simple command'
true
echo reached
__IN__
reached
__OUT__

test_o -e 0 'errexit: successful simple command' -e
true
echo reached
__IN__
reached
__OUT__

test_o -e 0 'noerrexit: failed simple command'
false
echo reached
__IN__
reached
__OUT__

test_O -e n 'errexit: failed simple command' -e
false
echo not reached
__IN__

test_o -e 0 'noerrexit: independent redirection error'
<_no_such_file_
echo reached
__IN__
reached
__OUT__

test_O -e n 'errexit: independent redirection error' -e
<_no_such_file_
echo not reached
__IN__

test_o -e 0 'noerrexit: redirection error on simple command'
echo not printed <_no_such_file_
echo reached
__IN__
reached
__OUT__

test_O -e n 'errexit: redirection error on simple command' -e
echo not printed <_no_such_file_
echo not reached
__IN__

test_o -e 0 'noerrexit: middle of pipeline'
false | false | true
echo reached
__IN__
reached
__OUT__

test_o -e 0 'errexit: middle of pipeline' -e
false | false | true
echo reached
__IN__
reached
__OUT__

test_o -e 0 'noerrexit: last of pipeline'
true | true | false
echo reached
__IN__
reached
__OUT__

test_O -e n 'errexit: last of pipeline' -e
true | true | false
echo not reached
__IN__

test_o -e 0 'noerrexit: negated pipeline'
! false | false | true
! true | true | false
echo reached
__IN__
reached
__OUT__

test_o -e 0 'errexit: negated pipeline' -e
! false | false | true
! true | true | false
echo reached
__IN__
reached
__OUT__

test_o -e 0 'noerrexit: initially failing and list'
false && ! echo not reached && ! echo not reached
echo reached
__IN__
reached
__OUT__

test_o -e 0 'errexit: initially failing and list' -e
false && ! echo not reached && ! echo not reached
echo reached
__IN__
reached
__OUT__

test_o -e 0 'noerrexit: finally failing and list'
true && true && false
echo reached
__IN__
reached
__OUT__

test_O -e n 'errexit: finally failing and list' -e
true && true && false
echo not reached
__IN__

test_o -e 0 'noerrexit: all succeeding and list'
true && true && true
echo reached
__IN__
reached
__OUT__

test_o -e 0 'errexit: all succeeding and list' -e
true && true && true
echo reached
__IN__
reached
__OUT__

test_o -e 0 'noerrexit: initially succeeding or list'
true || echo not reached || echo not reached
echo reached
__IN__
reached
__OUT__

test_o -e 0 'errexit: initially succeeding or list' -e
true || echo not reached || echo not reached
echo reached
__IN__
reached
__OUT__

test_o -e 0 'noerrexit: finally succeeding or list'
false || false || true
echo reached
__IN__
reached
__OUT__

test_o -e 0 'errexit: finally succeeding or list' -e
false || false || true
echo reached
__IN__
reached
__OUT__

test_o -e 0 'noerrexit: all failing or list'
false || false || false
echo reached
__IN__
reached
__OUT__

test_O -e n 'errexit: all failing or list' -e
false || false || false
echo not reached
__IN__

test_o -e 0 'noerrexit: subshell'
(echo reached 1; false; echo reached 2) | cat
(echo reached 3; false; echo reached 4)
echo reached 5
__IN__
reached 1
reached 2
reached 3
reached 4
reached 5
__OUT__

test_o -e n 'errexit: subshell' -e
(echo reached 1; false; echo not reached 2) | cat
(echo reached 3; false; echo not reached 4)
echo not reached 5
__IN__
reached 1
reached 3
__OUT__

test_o -e 0 'noerrexit: grouping'
{ echo reached 1; false; echo reached 2; } | cat
{ echo reached 3; false; echo reached 4; }
echo reached 5
__IN__
reached 1
reached 2
reached 3
reached 4
reached 5
__OUT__

test_o -e n 'errexit: grouping' -e
{ echo reached 1; false; echo not reached 2; } | cat
{ echo reached 3; false; echo not reached 4; }
echo not reached 5
__IN__
reached 1
reached 3
__OUT__

test_o -e 0 'noerrexit: for loop body'
for i in 1 2 3; do
    echo a $i
    test $i -ne 2
    echo b $i
done
echo reached
__IN__
a 1
b 1
a 2
b 2
a 3
b 3
reached
__OUT__

test_o -e n 'errexit: for loop body' -e
for i in 1 2 3; do
    echo a $i
    test $i -ne 2
    echo b $i
done
echo not reached
__IN__
a 1
b 1
a 2
__OUT__

test_o -e 0 'noerrexit: case body'
case a in a)
    echo reached 1
    false
    echo reached 2
esac
echo reached 3
__IN__
reached 1
reached 2
reached 3
__OUT__

test_o -e n 'errexit: case body' -e
case a in a)
    echo reached 1
    false
    echo not reached 2
esac
echo not reached 3
__IN__
reached 1
__OUT__

test_o -e 0 'noerrexit: if condition'
if false; true; then
    echo reached 1
else
    echo not reached
fi
echo reached 2
__IN__
reached 1
reached 2
__OUT__

test_o -e 0 'errexit: if condition' -e
if false; true; then
    echo reached 1
else
    echo not reached
fi
echo reached 2
__IN__
reached 1
reached 2
__OUT__

test_o -e 0 'noerrexit: elif condition'
if false; then
    :
elif false; true; then
    echo reached 1
else
    echo not reached
fi
echo reached 2
__IN__
reached 1
reached 2
__OUT__

test_o -e 0 'errexit: elif condition' -e
if false; then
    :
elif false; true; then
    echo reached 1
else
    echo not reached
fi
echo reached 2
__IN__
reached 1
reached 2
__OUT__

test_o -e 0 'noerrexit: then body'
if true; then echo reached 1; false; echo reached 2; fi
echo reached 3
__IN__
reached 1
reached 2
reached 3
__OUT__

test_o -e n 'errexit: then body' -e
if true; then echo reached 1; false; echo not reached 2; fi
echo not reached 3
__IN__
reached 1
__OUT__

test_o -e 0 'noerrexit: else body'
if false; then :; else echo reached 1; false; echo reached 2; fi
echo reached 3
__IN__
reached 1
reached 2
reached 3
__OUT__

test_o -e n 'errexit: else body' -e
if false; then :; else echo reached 1; false; echo not reached 2; fi
echo not reached 3
__IN__
reached 1
__OUT__

test_o -e 0 'noerrexit: while condition'
while false; do
    :
done
echo reached
__IN__
reached
__OUT__

test_o -e 0 'errexit: while condition' -e
while false; do
    :
done
echo reached
__IN__
reached
__OUT__

test_o -e 0 'noerrexit: while body'
while true; do
    echo reached 1
    false
    echo reached 2
    break
done
echo reached 3
__IN__
reached 1
reached 2
reached 3
__OUT__

test_o -e n 'errexit: while body' -e
while true; do
    echo reached 1
    false
    echo not reached 2
    break
done
echo not reached 3
__IN__
reached 1
__OUT__

test_o -e 0 'noerrexit: until condition'
until false; true; do
    :
done
echo reached
__IN__
reached
__OUT__

test_o -e 0 'errexit: until condition' -e
until false; true; do
    :
done
echo reached
__IN__
reached
__OUT__

test_o -e 0 'noerrexit: until body'
until false; do
    echo reached 1
    false
    echo reached 2
    break
done
echo reached 3
__IN__
reached 1
reached 2
reached 3
__OUT__

test_o -e n 'errexit: until body' -e
until false; do
    echo reached 1
    false
    echo reached 2
    break
done
echo reached 3
__IN__
reached 1
__OUT__

test_O -e n 'ignored failure in subshell' -e
( false && true; )
echo not reached
__IN__

test_o -e 0 'ignored failure in grouping' -e
{ false && true; }
echo reached
__IN__
reached
__OUT__

test_o -e 0 'ignored failure in if body' -e
if true; then false && true; fi
echo reached
__IN__
reached
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
