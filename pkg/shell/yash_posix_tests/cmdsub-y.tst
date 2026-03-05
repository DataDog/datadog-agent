# cmdsub-y.tst: yash-specific test of command substitution

# Seemingly meaningless comments like #` in this script are to work around
# syntax highlighting errors on some editors.

test_oE 'disambiguation with arithmetic expansion, single line'
echo $((echo foo); (echo bar))
__IN__
foo bar
__OUT__

test_oE 'disambiguation with arithmetic expansion, with here-document'
echo $((cat <<END
+
END
)
(echo - foo))
__IN__
+ - foo
__OUT__
#))

test_oE 'line-continuation within backquoted command substitution'
# Literal interpretation of XCU 2.6.3 implies that line-continuations within a
# backquoted command substitution should be parsed when the substitution is
# evaluated rather than when parsed. Many existing shells, however, do not do
# so...
echo `echo 'a\
b'`
__IN__
ab
__OUT__

test_Oe -e 2 'unclosed command substitution $()'
echo $(echo not reached
__IN__
syntax error: `)' is missing
__ERR__
#'
#`

test_Oe -e 2 'unclosed command substitution $(('
echo $((echo not reached
__IN__
syntax error: `))' is missing
__ERR__
#'
#`
#))

test_Oe -e 2 'unclosed command substitution $(()'
echo $((echo not reached)
__IN__
syntax error: `)' is missing
__ERR__
#'
#`

test_Oe -e 2 'unclosed command substitution ``'
echo `echo not reached
__IN__
syntax error: the backquoted command substitution is not closed
__ERR__
#`

test_Oe -e 2 'unclosed command substitution with escaped backquote'
echo `echo not reached \`
__IN__
syntax error: the backquoted command substitution is not closed
__ERR__
#`

test_oe -e 0 'unclosed double quotation in backquoted command substitution'
echo foo `echo "not reached`
__IN__
foo
__OUT__
command substitution:1: syntax error: the double quotation is not closed
__ERR__
#`
#"
#`

test_oe -e 0 'unclosed nested backquoted command substitution'
echo foo `echo \`not reached`
__IN__
foo
__OUT__
command substitution:1: syntax error: the backquoted command substitution is not closed
__ERR__
#`
#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
