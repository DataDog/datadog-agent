# bracket-y.tst: yash-specific test of the double-bracket command

if ! testee -c 'command -v [[' >/dev/null; then
    skip="true"
fi

test_OE -e 1 'single empty string primary'
[[ '' ]]
__IN__

test_OE -e 0 'single normal string primary'
[[ foo ]]
__IN__

# Note: zsh rejects -# and --
test_OE -e 0 'single dash-starting string primary'
[[ -# ]] && [[ -- ]] && [[ -no-such-operator ]]
__IN__

test_OE -e 0 'single quoted-operator string primary'
[[ -\f ]]
__IN__

test_OE -e 0 'single string primary expanding to operator'
op=-f
[[ $op ]]
__IN__

test_OE -e 0 'single unary primary -d'
[[ -d / ]] && ! [[ -d /dev/null ]]
__IN__

test_OE -e 0 'single unary primary -n'
[[ -n -n ]] && ! [[ -n """" ]]
__IN__

test_OE -e 0 'single unary primary with operator-looking operand'
[[ -n -eq ]] && [[ -n ! ]]
__IN__

test_OE -e 0 'single binary primary -eq'
[[ 0 -eq 0 ]] && ! [[ -1 -eq 1 ]]
__IN__

# Note: version number comparing primaries are unique to yash
test_OE -e 0 'single binary primary -vlt'
[[ 0.2.3 -vlt 0.10 ]] && ! [[ 01.2 -vlt 1.2 ]]
__IN__

test_OE -e 0 'single binary primary <'
[[ 0 < 1 ]] && ! [[ a < a ]]
__IN__

test_OE -e 0 'single binary primary >'
[[ 14.0 > 13.0 ]] && ! [[ foo > foo ]]
__IN__

test_OE -e 0 'literal pattern matching with binary primary ='
[[ foobar = f*b?r ]] && ! [[ foobar = "f*b?r" ]]
__IN__

# Note: zsh by default does not match expanded patterns
test_OE -e 0 'expanded pattern matching with binary primary ='
lhs='foobar' rhs='f*b?r'
[[ "$lhs" = $rhs ]] && ! [[ "$lhs" = "$rhs" ]]
__IN__

# Note: ksh renders reverse results for the first two
test_OE -e 0 'bracket pattern with binary primary ='
! [[ b = [a"-"c] ]] && [[ - = [a"-"c] ]] && ! [[ \\ = ["."] ]]
__IN__

test_OE -e 0 'literal pattern matching with binary primary =='
[[ foobar == f*b?r ]] && ! [[ foobar == "f*b?r" ]]
__IN__

test_OE -e 0 'expanded pattern matching with binary primary =='
lhs='foobar' rhs='f*b?r'
[[ "$lhs" == $rhs ]] && ! [[ "$lhs" == "$rhs" ]]
__IN__

test_OE -e 0 'literal pattern matching with binary primary !='
! [[ foobar != f*b?r ]] && [[ foobar != "f*b?r" ]]
__IN__

test_OE -e 0 'expanded pattern matching with binary primary !='
lhs='foobar' rhs='f*b?r'
! [[ "$lhs" != $rhs ]] && [[ "$lhs" != "$rhs" ]]
__IN__

# Note: yash's behavior is not consistent with other shells
test_OE -e 0 'unquoted backslash is special in pattern'
bs='\'
[[ $bs = $bs$bs ]] && [[ $bs == $bs$bs ]] && [[ $bs != $bs ]]
__IN__

# Note: mksh does not support regex
test_OE -e 0 'literal regex matching with binary primary =~'
[[ abc123xyz =~ c[[:digit:]]*x ]] && ! [[ abc =~ b'*' ]]
__IN__

test_OE -e 0 'expanded pattern matching with binary primary =~'
lhs='abc123xyz' rhs='c[[:digit:]]*x'
[[ "$lhs" =~ $rhs ]] && ! [[ "$lhs" =~ "$rhs" ]]
__IN__

test_OE -e 0 'dollars with binary primary =~'
[[ abc =~ c$ ]] && ! [[ abc =~ ^c$ ]]
__IN__

test_OE -e 0 'vertical bars with binary primary =~'
[[ a =~ a|b ]] && [[ b =~ a|b|c ]]
__IN__

test_OE -e 0 'successive vertical bars with binary primary =~' -n
# Empty branches of | cause undefined behavior as per POSIX, but it should not
# be a syntax error.
[[ '' =~ a|| ]]
[[ '' =~ ||a ]]
__IN__

test_OE -e 0 'parentheses with binary primary =~'
[[ a =~ (a) ]] && [[ '  (  ' =~ ( ( \( ) ) ]]
__IN__

test_OE -e 0 'combination of specials with binary primary =~'
a=1 b=2 c=3
[[ 123 =~ $a($b|\\)$c`` ]]
__IN__

# Note: zsh rejects this
test_OE -e 0 'escaping with binary primary =~ (backslash)'
[[ \\ =~ \\ ]] && ! [[ \\ =~ \\\\ ]]
__IN__

test_OE -e 0 'escaping with binary primary =~ (parentheses)'
[[ \(a\) =~ \(a\) ]] && ! [[ a =~ \(a\) ]]
__IN__

test_OE -e 0 'escaping with binary primary =~ (vertical bar)'
[[ \| =~ \| ]] && ! [[ '' =~ \| ]]
__IN__

test_OE -e 0 'escaping with binary primary =~ (braces)'
[[ a\{3\} =~ a\{3\} ]] && ! [[ aaa =~ a\{3\} ]]
__IN__

# Note: ksh and zsh differ in these cases
test_OE -e 0 'quoting with binary primary =~'
[[ ".+" =~ ".+" ]] && ! [[ a =~ ".+" ]]
__IN__

test_OE -e 0 'expanded specials with binary primary =~ (w/o quotes)'
a='*' b='|' bb='\|' p='(a|b)'
[[ abc =~ ab${a}c ]] && [[ a =~ a${b}b ]] && ! [[ a =~ a${bb}b ]] &&
    [[ a =~ $p ]] && [[ a =~ ($(echo a)) ]]
__IN__

# Note: zsh differs in most of these cases
test_OE -e 0 'expanded specials with binary primary =~ (w/ quotes)'
a='*' b='|' bb='\|' p='(a|b)'
! [[ abc =~ "ab${a}c" ]] && ! [[ a =~ "a${b}b" ]] && ! [[ a =~ "a${bb}b" ]] &&
    ! [[ a =~ "$p" ]]
__IN__

# Note: ksh and zsh behaves differently for some of the below
test_OE -e 0 'bracket pattern with binary primary =~'
[[ b =~ [a"-"c] ]] && ! [[ - =~ [a"-"c] ]] &&
[[ 'a*c' =~ 'a*c' ]] && [[ "a<b" =~ "a<b" ]] &&
! [[ \\ =~ ["."] ]] && [[ \\ =~ [[.\\.]] ]] &&
[[ x] =~ [^"]]]" ]] && [[ a+ =~ [a"[:alnum:]]+" ]]
__IN__

# Note: bash returns exit status of 2 and zsh prints an error message
test_OE -e 1 'ill-formed regex with binary primary =~'
[[ foo =~ * ]]
__IN__

test_OE -e 0 'single binary primary with operator-looking operand'
[[ -eq = -eq ]] && [[ \-f = -f ]] && [[ ''= = = ]] && [[ \! = ! ]]
__IN__

test_OE -e 0 'parentheses'
[[ ( foo ) ]] && [[ ( -n -n ) ]] && [[ ( 100 -eq 100 ) ]]
__IN__

test_OE -e 0 'negating empty string primary'
[[ ! '' ]]
__IN__

test_OE -e 1 'negating non-empty string primary'
[[ ! foo ]]
__IN__

test_OE -e 0 'negating unary primary (false -> true)'
[[ ! -n '' ]]
__IN__

test_OE -e 1 'negating unary primary (true -> false)'
[[ ! -n foo ]]
__IN__

test_OE -e 0 'negating binary primary (false -> true)'
[[ ! a = b ]]
__IN__

test_OE -e 1 'negating binary primary (true -> false)'
[[ ! a = a ]]
__IN__

test_OE -e 0 'true && true'
[[ foo && bar = bar ]]
__IN__

test_OE -e 1 'true && false'
[[ foo && bar != bar ]]
__IN__

test_OE -e 1 'false && ...'
[[ foo != foo && $(echo not reached >&2) ]]
__IN__

test_OE -e 0 'true || ...'
[[ foo = foo || $(echo not reached >&2) ]]
__IN__

test_OE -e 0 'false || true'
[[ foo != foo || -n foo ]]
__IN__

test_OE -e 1 'false || false'
[[ foo != foo || '' ]]
__IN__

test_OE -e 1 '! has higher precedence than &&'
[[ ! -z foo && foo != foo ]]
__IN__

test_OE -e 0 '! has higher precedence than ||'
[[ ! -n foo || foo = foo ]]
__IN__

test_OE -e 0 '&& has higher precedence to the left of ||'
[[ -z foo && 0 -eq 0 || bar = bar ]]
__IN__

test_OE -e 0 '&& has higher precedence to the left of ||'
[[ foo = foo || 1 -eq 1 && -z bar ]]
__IN__

# Note: other shells (bash, ksh, mksh and zsh) don't accept this
test_OE -e 0 'IO_NUMBER is not special between [[ and ]]'
[[ 0<1 ]]
__IN__

test_Oe -e 2 'expansion error (string)'
unset v
eval '[[ $(echo ok >&2) || ${v?!!!} || $(echo not reached >&2) ]]'
echo not reached
__IN__
ok
eval: v: !!!
__ERR__

test_Oe -e 2 'expansion error (unary)'
unset v
eval '[[ -n ${v?X} ]]'
echo not reached
__IN__
eval: v: X
__ERR__

test_Oe -e 2 'expansion error (binary, left hand side)'
unset v
eval '[[ ${v?X} = v ]]'
echo not reached
__IN__
eval: v: X
__ERR__

test_Oe -e 2 'expansion error (binary, right hand side)'
unset v
eval '[[ v = ${v?X} ]]'
echo not reached
__IN__
eval: v: X
__ERR__

test_Oe -e 2 'syntax error: empty [[ ]]'
[[ ]]
__IN__
syntax error: conditional expression is missing or incomplete between `[[' and `]]'
__ERR__
#'
#`
#'
#`

# Note: ksh and mksh accept this which bash and zsh don't
test_Oe -e 2 'syntax error: single unquoted ]]'
[[ ]] ]]
__IN__
syntax error: conditional expression is missing or incomplete between `[[' and `]]'
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'syntax error: newline after [['
[[
__IN__
syntax error: unexpected linebreak in the middle of the [[ ... ]] command
__ERR__

test_Oe -e 2 'syntax error: newline in [[ ]]'
[[ foo
__IN__
syntax error: `]]' is missing
__ERR__
#'
#`

test_Oe -e 2 'syntax error: missing operand for unary primary'
[[ -f ]]
__IN__
syntax error: conditional expression is missing or incomplete between `[[' and `]]'
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'syntax error: missing right-hand-side operand for binary primary'
[[ foo = ]]
__IN__
syntax error: conditional expression is missing or incomplete between `[[' and `]]'
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'syntax error: redundant word'
[[ foo bar ]]
__IN__
syntax error: invalid word `bar' between `[[' and `]]'
__ERR__
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'syntax error: -a is not binary primary'
[[ foo -a bar ]]
__IN__
syntax error: invalid word `-a' between `[[' and `]]'
__ERR__
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'syntax error: -o is not binary primary'
[[ foo -o bar ]]
__IN__
syntax error: invalid word `-o' between `[[' and `]]'
__ERR__
#'
#`
#'
#`
#'
#`

test_Oe -e 2 'syntax error: invalid operator'
[[ ; ]]
__IN__
syntax error: `;' is not a valid operand in the conditional expression
__ERR__
#'
#`

test_Oe -e 2 'syntax error: newline after ( in [[ ]]'
[[ (
__IN__
syntax error: unexpected linebreak in the middle of the [[ ... ]] command
__ERR__

test_Oe -e 2 'syntax error: newline after string primary in ( in [[ ]]'
[[ ( foo
__IN__
syntax error: `)' is missing
__ERR__
#'
#`

test_Oe -e 2 'syntax error: missing ) before ]]'
[[ ( foo ]]
__IN__
syntax error: `)' is missing
__ERR__
#'
#`

test_Oe -e 2 'syntax error: empty ( ) in [[ ]]'
[[ ( ) ]]
__IN__
syntax error: `)' is not a valid operand in the conditional expression
__ERR__
#'
#`

test_oE -e 0 '[[ is keyword'
command -v [[
command -V [[
__IN__
[[
[[: a shell keyword
__OUT__

# Note: ]] is keyword in bash
test_Oe -e 1 ']] is not keyword'
PATH=
command -V ]]
__IN__
command: no such command `]]'
__ERR__
#'
#`

(
posix="true"

test_oE -e 0 '[[ is keyword (POSIX)'
command -v [[
command -V [[
__IN__
[[
[[: a shell keyword
__OUT__

test_Oe -e 2 'syntax error: [[ is not supported in POSIX mode'
[[ foo ]]
__IN__
syntax error: The [[ ... ]] syntax is not supported in the POSIXly-correct mode
__ERR__

)

# vim: set ft=sh ts=8 sts=4 sw=4 et:
