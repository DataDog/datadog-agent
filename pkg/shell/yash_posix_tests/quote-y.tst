# quote-y.tst: yash-specific test of quoting

(
posix="true"

# POSIX does not imply quote removal to be performed on the error message but
# many existing shells do it.
test_Oe -e 2 'quotes in substitution of expansion ${a?b}'
eval '${u?\ \!\$x\%\&\(\)\*\+\,\-\.\/\{\|\} \# \"x\" \'\''x\'\''\?\\\`\`"x"'\''y'\''}'
__IN__
eval: u:  !$x%&()*+,-./{|} # "x" 'x'?\``xy
__ERR__

test_Oe -e 2 'quotes in substitution of expansion ${a?b} in double quotes'
eval '"${u?\ \!\$x\%\&\(\)\*\+\,\-\.\/\{\|\} \# \"x\" \'\''x\'\''\?\\\\\`\`"x"'\''y'\''}"'
__IN__
eval: u: \ \!$x\%\&\(\)\*\+\,\-\.\/\{\|} \# "x" \'x\'\?\\``x'y'
__ERR__

)

test_oE 'null character in dollar-single-quotes'
printf '%s\n' a$'b\0c'd w$'x\x0y'z 1$'2\c@3'4
__IN__
abd
wxz
12?34
__OUT__

test_oE 'too large octal escape in dollar-single-quotes'
printf '%s\n' $'\777'
__IN__
?
__OUT__

test_oE 'no dollar-single-quotes inside double quotes'
null=
printf '%s\n' "$'\x20$null'"
__IN__
$'\x20'
__OUT__

test_oE 'backslash preceding EOF is ignored'
"$TESTEE" -c 'printf "[%s]\n" 123\'
__IN__
[123]
__OUT__

test_oE 'line continuation in function definition'
\
f\
u\
n\
c\
t\
i\
o\
\
n\
	\
f"u\
n"c \
(\
 )\
\
{ echo foo; }
func
__IN__
foo
__OUT__

test_oE 'line continuation in parameter expansion'
f=foo
# echo ${#?} ${${f}} ${f[1,2]:+x}
echo \
$\
{\
\
#\
\
?\
\
} $\
\
{\
\
$\
\
{\
\
f\
\
}\
\
} $\
{\
f\
\
[\
\
1\
\
,\
\
2\
\
]\
\
:\
\
+\
\
x\
\
}
__IN__
1 foo x
__OUT__

test_Oe -e 2 'unclosed single quotation'
echo 'foo
-
__IN__
syntax error: the single quotation is not closed
__ERR__
#'

test_Oe -e 2 'unclosed double quotation (direct)'
echo "foo
__IN__
syntax error: the double quotation is not closed
__ERR__
#"

test_Oe -e 2 'unclosed double quotation (in parameter expansion)'
echo ${foo-"bar}
__IN__
syntax error: the double quotation is not closed
syntax error: `}' is missing
__ERR__
#'
#`
#"

# vim: set ft=sh ts=8 sts=4 sw=4 et:
