# async-y.tst: yash-specific test of asynchronous lists

test_Oe -e 2 'no command before ;'
;
__IN__
syntax error: a command is missing before `;'
__ERR__
#'`

test_Oe -e 2 'no command before &'
&
__IN__
syntax error: a command is missing before `&'
__ERR__
#'`

test_Oe -e 2 'no separator between commands'
if true; then echo 1; fi if true; then echo 2; fi
__IN__
syntax error: `;' or `&' is missing
__ERR__
#'`#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
