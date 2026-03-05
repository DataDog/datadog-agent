# alias-y.tst: yash-specific test of aliases

setup 'set -e'

test_OE -e 0 'yash has no predefined aliases'
alias
__IN__

test_oE 'multi-line alias'
alias e='echo 1
echo 2'
e 3
__IN__
1
2 3
__OUT__

test_oE 'alias in command substitution'
alias e=:
func() {
    alias e=echo
    echo "$(e not_printed)"
}
func
__IN__

__OUT__

test_Oe -e 2 'semicolon after newline (for)'
alias -g s=';'
for i
s do :; done
__IN__
syntax error: `;' cannot appear on a new line
__ERR__
#'
#`

(
if ! testee -c 'command -v [[' >/dev/null; then
    skip="true"
fi

test_OE -e 0 'alias substitution after [['
# "!" is not keyword between "[[" and "]]"
alias -g !=
[[ ! ! ! foo ]] && [[ ! foo = foo ]] && [[ ! -n foo ]] &&
    [[ ! ( ! foo ! = ! foo ! ) ! && ! foo ! ]] && [[ ! '' ! || ! foo ! ]]
__IN__

)

test_oE 'alias substitution after function keyword'
alias fn='function ' f=g
fn f { echo F; }
g
__IN__
F
__OUT__

test_oE 'alias substitution after function name with grouping'
alias fn='function f ' p='()' g='{ echo G; }'
fn p { echo F; }
f
fn g
f
__IN__
F
G
__OUT__

test_oE 'alias substitution after function name with empty parentheses body'
alias fn='function f ' p='() '
fn p p
{ echo this is not function body; }
__IN__
this is not function body
__OUT__

test_oE 'alias substitution after function open parenthesis'
alias fn='function f (' p=')'
fn p { echo F; }
f
__IN__
F
__OUT__

test_oE 'alias substitution after function parentheses'
alias fn='function f() ' g='{ echo G; }'
fn g
f
__IN__
G
__OUT__

test_oE -e 0 'global aliases'
alias -g A=a B=b C=c -- ---=-
echo C B A -A- -B- -C- \A "B" 'C' ---
__IN__
c b a -A- -B- -C- A B C -
__OUT__

test_oE -e 0 'global alias (substituting to line continuation)'
alias --global eeee='echo\
'
eeee eeee
__IN__
echo
__OUT__

test_oE -e 0 'global alias (substituting to single-quotation)'
alias --global sq="'"
printf '[%s]\n' sq  A   ' sq;'
__IN__
[  A   ]
[;]
__OUT__

test_oE -e 0 'global alias (substituting to pipeline)'
alias -g pipe_cat='| cat'
echo A pipe_cat -
__IN__
A
__OUT__

test_oE -e 0 'global alias (substituting to semicolon)'
alias -g semicolon=';'
echo A semicolon echo A
__IN__
A
A
__OUT__

test_oE -e 0 'global alias (substituting to parenthesis)'
alias -g l='(' r=')'
l echo A r
__IN__
A
__OUT__

test_OE -e 0 'global alias (substituting to redirection)'
alias -g I='</dev/null' O='>/dev/null'
echo 1 O
echo 2 | cat I
(echo 3) O
{ echo 4; } O
if :; then echo 5; fi O
if if :; then :; fi then if :; then echo 6; fi fi O
if if :; then :; fi then if :; then echo 7; fi O fi
if if echo 8; then :; fi O then if :; then :; fi fi
__IN__

test_oE -e 0 'global alias (ending with blank)'
alias a=unexpected_substitution
alias -g echo='echo '
echo a
__IN__
a
__OUT__

test_oE -e 0 'global aliases are ignored in POSIX mode'
alias -g a=unexpected_substitution
set -o posix
echo a
__IN__
a
__OUT__

test_oE -e 0 'global aliases apply just once'
alias -g a='a a '
echo a
alias echo='echo '
echo a
__IN__
a a
a a
__OUT__

test_oE -e 0 'printing all aliases (without -p)'
alias a=A b=B c=C
alias -g x=X y=Y z=Z
alias
__IN__
a=A
b=B
c=C
x=X
y=Y
z=Z
__OUT__

test_oE -e 0 'printing all aliases (hyphen & quotation, without -p)'
alias -- -="'"
alias -g -- ---='"'
alias
__IN__
-=\'
---='"'
__OUT__

test_oE -e 0 'printing all aliases (with -p)'
alias a=A b=B c=C
alias -g x=X y=Y z=Z
alias -p
__IN__
alias a=A
alias b=B
alias c=C
alias -g x=X
alias -g y=Y
alias -g z=Z
__OUT__

test_oE -e 0 'printing all aliases (with --prefix)'
alias a=A b=B c=C
alias -g x=X y=Y z=Z
alias --prefix
__IN__
alias a=A
alias b=B
alias c=C
alias -g x=X
alias -g y=Y
alias -g z=Z
__OUT__

test_oE -e 0 'printing all aliases (hyphen & quotation, with -p)'
alias -- -="'"
alias -g -- ---='"'
alias -p
__IN__
alias -- -=\'
alias -g -- ---='"'
__OUT__

(
setup - <<\END
alias -- e=echo echo='echo a'
alias -g -- a=A ---=-----
END

test_oE -e 0 'printing specific global alias (without -p)'
alias \a e
__IN__
a=A
e=echo
__OUT__

test_oE -e 0 'printing specific global alias (with -p)'
alias -p \a e
__IN__
alias -g a=A
alias e=echo
__OUT__

test_OE -e 0 '"unalias -a" removes global aliases'
unalias -a
alias
__IN__

test_Oe -e n 'alias built-in invalid option'
alias --no-such-option
__IN__
alias: `--no-such-option' is not a valid option
__ERR__
#'
#`

test_Oe -e n 'alias built-in printing non-existing alias'
alias no_such_alias
__IN__
alias: no such alias `no_such_alias'
__ERR__
#'
#`

test_O -d -e n 'alias built-in printing to closed stream without operands'
alias >&-
__IN__

test_O -d -e n 'alias built-in printing to closed stream with operands'
alias \a e >&-
__IN__

test_O -d -e n 'alias built-in printing to closed stream with -p'
alias -p >&-
__IN__

test_O -d -e n 'alias built-in printing to closed stream with -p & operands'
alias -p \a e >&-
__IN__

test_Oe -e n 'unalias built-in invalid option'
unalias --no-such-option
__IN__
unalias: `--no-such-option' is not a valid option
__ERR__
#'
#`

test_Oe -e n 'unalias built-in invalid combination of -a and operands'
unalias -a e
__IN__
unalias: no operand is expected
__ERR__

test_Oe -e n 'unalias built-in missing operand'
unalias
__IN__
unalias: this command requires an operand
__ERR__

test_Oe -e n 'unalias built-in printing non-existing alias'
unalias no_such_alias
__IN__
unalias: no such alias `no_such_alias'
__ERR__
#'
#`

)

# vim: set ft=sh ts=8 sts=4 sw=4 et:
