# for-y.tst: yash-specific test of for loop

(
posix="true"

test_oE 'assignment is persistent and global (-o POSIX)'
f() for v in function; do :; done
f
echo $v
__IN__
function
__OUT__

test_Oe -e 2 'empty for loop (single line, -o POSIX)'
for i do done
__IN__
syntax error: commands are missing between `do' and `done'
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'empty for loop (multi-line, -o POSIX)'
for i do
done
__IN__
syntax error: commands are missing between `do' and `done'
__ERR__
#'
#`
#'
#`

)

test_oE 'assignment is persistent and local (+o POSIX, -o forlocal)'
f() {
    for v in function; do :; done
    echo $v
}
v=foo
f
echo $v
__IN__
function
foo
__OUT__

test_oE 'assignment is persistent and global (+o POSIX, +o forlocal)'
set -o noforlocal
unset -v i
fn() { for i in a b c; do : ; done; }
fn
echo "${i-UNSET}"
__IN__
c
__OUT__

test_oE 're-assignment to loop variable during loop'
for v in A B
do
    echo $v
    v=X
    echo $v
done
__IN__
A
X
B
X
__OUT__

test_oE 'effect of empty for loop (-o POSIX)'
echo 1
for i in i
do done
echo 2
__IN__
1
2
__OUT__

test_OE -e 13 'exit status of empty for loop (-o POSIX)'
sh -c 'exit 13'
for i in i
do
done
__IN__

test_Oe -e 2 'do without for'
do echo 1; done
__IN__
syntax error: encountered `do' without a matching `for', `while', or `until'
__ERR__
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'done without for ... do'
done
__IN__
syntax error: encountered `done' without a matching `do'
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'semicolon after newline'
for i
; do :; done
__IN__
syntax error: `;' cannot appear on a new line
__ERR__
#'
#`

test_Oe -e 2 'words followed by done'
for i in a b c; done
__IN__
syntax error: `do' is missing
__ERR__
#'
#`

test_Oe -e 2 'variable name followed by done'
for i done
__IN__
syntax error: `do' is missing
__ERR__
#'
#`

test_Oe -e 2 'for followed by EOF'
for
__IN__
syntax error: an identifier is required after `for'
syntax error: `do' is missing
syntax error: `done' is missing
__ERR__
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'variable name followed by EOF'
for i
__IN__
syntax error: `do' is missing
syntax error: `done' is missing
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'in followed by EOF'
for i in
__IN__
syntax error: `do' is missing
syntax error: `done' is missing
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'do followed by EOF (without in)'
for i do
__IN__
syntax error: `done' is missing
__ERR__
#'
#`

test_Oe -e 2 'do followed by EOF (with in)'
for i in
do
__IN__
syntax error: `done' is missing
__ERR__
#'
#`

test_Oe -e 2 '{ for }'
{ for }
__IN__
syntax error: `}' is not a valid identifier
syntax error: `do' is missing
syntax error: `done' is missing
syntax error: `}' is missing
__ERR__
#'
#`
#'
#`
#'
#`
#'
#`

test_Oe -e 2 '{ for _ }'
{ for _ }
__IN__
syntax error: `do' is missing
syntax error: encountered `}' without a matching `{'
syntax error: (maybe you missed `done'?)
__ERR__
#'
#`
#'
#`
#'
#`
#'
#`

test_Oe -e 2 '{ for _ in }'
{ for _ in }
__IN__
syntax error: `do' is missing
syntax error: `done' is missing
syntax error: `}' is missing
__ERR__
#'
#`
#'
#`
#'
#`

test_Oe -e 2 '{ for _ in; }'
{ for _ in; }
__IN__
syntax error: `do' is missing
syntax error: encountered `}' without a matching `{'
syntax error: (maybe you missed `done'?)
__ERR__
#'
#`
#'
#`
#'
#`

test_Oe -e 2 '{ for _ do }'
{ for _ do }
__IN__
syntax error: encountered `}' without a matching `{'
syntax error: (maybe you missed `done'?)
__ERR__
#'
#`
#'
#`

test_Oe -e 2 '{ for _ in; do }'
{ for _ in; do }
__IN__
syntax error: encountered `}' without a matching `{'
syntax error: (maybe you missed `done'?)
__ERR__
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'invalid variable name'
for = do echo not reached; done
__IN__
syntax error: `=' is not a valid identifier
__ERR__
#'
#`

test_Oe -e 2 'invalid word separator'
for i in 1 2& do echo $i; done
__IN__
syntax error: `do' is missing
syntax error: a command is missing before `&'
syntax error: encountered `do' without a matching `for', `while', or `until'
syntax error: (maybe you missed `done'?)
__ERR__
#'
#`
#'
#`
#'
#`
#'
#`
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'redirection in words'
for i in a </dev/null b; do echo $i; done
__IN__
syntax error: `do' is missing
syntax error: encountered `do' without a matching `for', `while', or `until'
syntax error: (maybe you missed `done'?)
__ERR__
#'
#`
#'
#`
#'
#`
#'
#`
#'
#`
#'
#`

test_oE 'semicolon after variable name (single line, +o POSIX)' -s A B
for i; do echo $i; done
__IN__
A
B
__OUT__

test_oE 'semicolon after variable name (mulit-line, +o POSIX)' -s A B
for i;
do echo $i; done
__IN__
A
B
__OUT__

test_O -d -e 2 'read-only variable'
readonly v=readonly
for v in 1; do
    echo not reached
done
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
