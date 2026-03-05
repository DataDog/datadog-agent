# grouping-p.tst: test of grouping commands for any POSIX-compliant shell

posix="true"

mkfifo fifo1

test_oE 'effect of subshell'
a=1
(a=2; echo $a; exit; echo not reached)
echo $a
__IN__
2
1
__OUT__

test_x -e 23 'exit status of subshell'
(true; exit 23)
__IN__

test_oE 'redirection on subshell'
(echo 1; echo 2; echo 3; echo 4) >sub_out
(tail -n 2) <sub_out
__IN__
3
4
__OUT__

test_oE 'subshell ending with semicolon'
(echo foo;)
__IN__
foo
__OUT__

test_oE 'subshell ending with asynchronous list'
(echo foo >fifo1&)
cat fifo1
__IN__
foo
__OUT__

test_oE 'newlines in subshell'
(
echo foo
)
__IN__
foo
__OUT__

test_oE 'effect of brace grouping'
a=1
{ a=2; echo $a; exit; echo not reached; }
echo not reached
__IN__
2
__OUT__

test_x -e 29 'exit status of brace grouping'
{ true; sh -c 'exit 29'; }
__IN__

test_oE 'redirection on brace grouping'
{ echo 1; echo 2; echo 3; echo 4; } >brace_out
{ tail -n 2; } <brace_out
__IN__
3
4
__OUT__

test_oE 'brace grouping ending with semicolon'
{ echo foo; }
__IN__
foo
__OUT__

test_oE 'brace grouping ending with asynchronous list'
{ echo foo >fifo1&}
cat fifo1
__IN__
foo
__OUT__

test_oE 'newlines in brace grouping'
{
echo foo
}
__IN__
foo
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
