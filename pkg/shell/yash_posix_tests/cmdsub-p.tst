# cmdsub-p.tst: test of command substitution for any POSIX-compliant shell

posix="true"

> dummyfile
setup -d

test_oE -e 0 'result of command substitution'
a=$(echo a) && bracket $a`echo b`
__IN__
[ab]
__OUT__

test_oE 'command substitution executes in subshell'
a=a
b=$(a=x; echo b)
bracket $a$b
__IN__
[ab]
__OUT__

test_oE 'trailing newlines are removed'
a=$(printf 'x\ny') b=$(printf 'x\ny\n') c=$(printf 'x\n\ny\n\n\n\n')
bracket "$a" "$b" "$c"
__IN__
[x
y][x
y][x

y]
__OUT__

test_oE 'stdin is not redirected'
echo a | echo $(cat)
__IN__
a
__OUT__

test_oe -e 0 'stderr is not redirected'
bracket "$(echo x >&2)"
__IN__
[]
__OUT__
x
__ERR__

test_oE 'field splitting on result of command substitution'
bracket $(printf 'a\n\nb')
__IN__
[a][b]
__OUT__

test_oE 'backslash in backquotes / nested backquotes'
echoraw `echoraw \`echoraw x\``
echoraw `echoraw '\$y'`
echoraw `printf '%s\n' \\\\`
__IN__
x
$y
\
__OUT__

test_oE 'quotations in backquotes'
echoraw `echoraw "a"'b'`
echoraw `echoraw \$ "\$" '\$'`
echoraw `echoraw \\\\ "\\\\" '\\\\'`
echoraw `echoraw \" "\"" '\"'`
echoraw `echoraw \' "\'"`
echoraw `echoraw \`echo a\` "\`echo b\`" '\`echo c\`'`
__IN__
ab
$ $ $
\ \ \\
" " \"
' \'
a b `echo c`
__OUT__

test_oE 'quotations in backquotes in double quotes'
echoraw "`echoraw "a"'b'`"
echoraw "`echoraw \$ "\$" '\$'`"
echoraw "`echoraw \\\\ "\\\\" '\\\\'`"
echoraw "`echoraw \"1\"`"
echoraw "`echoraw \'2\'`"
echoraw "`echoraw \`echo a\` "\`echo b\`" '\`echo c\`'`"
__IN__
ab
$ $ $
\ \ \\
1
'2'
a b `echo c`
__OUT__

test_oE 'quotations in backquotes in here-document'
cat <<END
`echoraw \"1\"`
" `echoraw \"2\"` "
END
__IN__
1
" 2 "
__OUT__

test_oE 'quotations in command substitution'
echoraw "$(echoraw ")\$"')\$'\)\$)"
__IN__
)$)\$)$
__OUT__

test_oE 'comment in command substitution'
echoraw "$(
echo a # ) comment
)"
__IN__
a
__OUT__

test_oE 'case command in command substitution'
echoraw "$(
case a in
(a) echo x;;
 *) echo not reached;;
esac
)"
__IN__
x
__OUT__

test_oE 'here-document in command substitution'
echoraw "$(cat <<\END
foo)
END
)"
__IN__
foo)
__OUT__

test_oE 'command substitution between here-document operator and body'
cat <<\OUTER; echoraw "$(cat <<\INNER
inner
INNER
)"
outer
OUTER
__IN__
outer
inner
__OUT__

test_oE 'result of command substitution is not subject to further expansion'
a=A HOME=home
echoraw $(echoraw '~/$a$()$((1))``')
echoraw "$(echoraw '~/$a$()$((1))``')"
__IN__
~/$a$()$((1))``
~/$a$()$((1))``
__OUT__

test_oE 'field splitting on result of command substitution'
bracket $(echoraw 'A B  C')
bracket "$(echoraw 'A B  C')"
__IN__
[A][B][C]
[A B  C]
__OUT__

test_oE 'pathname expansion on result of command substitution'
bracket $(echoraw 'dumm*ile')
bracket "$(echoraw 'dumm*ile')"
__IN__
[dummyfile]
[dumm*ile]
__OUT__

test_oE 'nested command substitutions'
echo $( (echo $(echo $(echo x))))
__IN__
x
__OUT__

# This test case is based on rationale of POSIX. The shell reaches the end of
# script before finding the end of arithmetic expansion and reports a syntax
# error, even if it could have been parsed as a well-formed command
# substitution.
test_O -d -e n 'ambiguity with arithmetic expansion, missing many )s'
echoraw $((cat <<EOF
+((((
EOF
) && (
cat <<EOF
+
EOF
))
__IN__
#))))

test_O -d -e n 'ambiguity with arithmetic expansion, missing one )' -c \
'echo $((cat <<EOF
+(
EOF
))'
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
