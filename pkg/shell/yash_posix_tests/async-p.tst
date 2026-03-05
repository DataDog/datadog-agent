# async-p.tst: test of asynchronous lists for any POSIX-compliant shell

posix="true"

test_o 'synchronous lists separated by semicolons'
echo 1; echo 2;echo 3;
echo 4

echo 5
__IN__
1
2
3
4
5
__OUT__

test_o 'asynchronous lists separated by ampersands'
echo& echo &echo&
wait
__IN__



__OUT__

test_o 'asynchronous commands run asynchronously'
# Note the blocking nature of opening a FIFO
mkfifo fifo1 fifo2
echo foo >fifo1 &
cat fifo1 >fifo2 &
cat fifo2 &
wait $!
__IN__
foo
__OUT__

test_o 'asynchronous command runs in subshell'
a=1
{ a=2; echo $a; }&
wait $!
echo $a
__IN__
2
1
__OUT__

test_oE 'stdin of asynchronous list is null without job control' +m
cat& wait
echo this line should not be consumed by cat
__IN__
this line should not be consumed by cat
__OUT__

echo foo > file

test_OE 'stdin of asynchronous list is null even if already redirected' +m -c '
exec < file
cat <&0 & wait
'
# In this test case, the script is given as a command line argument
# to prevent the test from being disrupted by the redirection.
__IN__

test_oE 'stdin of asynchronous list is null for first command only' +m
cat - file | cat | cat & wait
exit
__IN__
foo
__OUT__

test_o 'exit status of asynchronous list'
true&
echo $?
false&
echo $?
__IN__
0
0
__OUT__

test_o 'asynchronous and-or lists'
a=1
a=2 && echo $a&
wait
echo $a
__IN__
2
1
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
