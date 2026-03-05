# pipeline-y.tst: yash-specific test of pipeline

test_OE -e 42 '! with return'
f() { ! return 42; }
f
__IN__

test_Oe -e 2 'no command before |'
| echo foo
__IN__
syntax error: a command is missing before `|'
__ERR__
#`

test_Oe -e 2 'no command after |'
echo foo |
__IN__
syntax error: a command is missing at the end of input
__ERR__
#`

test_Oe -e 2 '| followed by !'
echo foo | ! cat
__IN__
syntax error: `!' cannot be used as a command name
__ERR__
#`

test_Oe -e 2 '! followed by !'
! ! echo ok
__IN__
syntax error: `!' cannot be used as a command name
__ERR__
#`

test_Oe -e 2 '| followed by |'
echo foo | | cat
__IN__
syntax error: a command is missing before `|'
__ERR__
#`

test_OE -e 0 '! immediately followed by ( (non-POSIX)'
!(false)
__IN__

test_Oe -e 2 '! immediately followed by ( (POSIX)' --posix
!(echo not printed)
__IN__
syntax error: ksh-like extended glob pattern `!(...)' is not supported
__ERR__
#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
