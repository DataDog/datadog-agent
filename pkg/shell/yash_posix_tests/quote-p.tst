# quote-p.tst: test of quoting for any POSIX-compliant shell

posix="true"

setup -d

test_oE 'backslash (not preceding newline)'
bracket \ \!\$x\%\&\(\)\*\+\,\-\.\/ \# \"x\" \'x\'
bracket \0\1\2\3\4\5\6\7\8\9\:\;\<\=\>\?
bracket \@\A\B\C\D\E\F\G\H\I\J\K\L\M\N\O\P\Q\R\S\T\U\V\W\X\Y\Z\[\]\^\_ \\ \\\\
bracket \a\b\c\d\e\f\g\h\i\j\k\l\m\n\o\p\q\r\s\t\u\v\w\x\y\z\{\|\}\~ \`\`
__IN__
[ !$x%&()*+,-./][#]["x"]['x']
[0123456789:;<=>?]
[@ABCDEFGHIJKLMNOPQRSTUVWXYZ[]^_][\][\\]
[abcdefghijklmnopqrstuvwxyz{|}~][``]
__OUT__

test_oE 'line continuation in normal word'
bracket 123\
456\
\
789 \
ABC\
 DEF
__IN__
[123456789][ABC][DEF]
__OUT__

test_OE -e 0 'line continuation in reserved word !'
\
!\
 false
__IN__

test_oE 'line continuation in reserved words { }'
\
{\
 echo 1
\
}\
||
\
{\
 echo 2
\
}\

__IN__
1
__OUT__

test_oE 'line continuation in reserved words case in esac and operator ;;'
\
c\
a\
s\
e\
 a \
i\
n\
 (a) echo 1\
;\
;\
e\
s\
a\
c\

__IN__
1
__OUT__

test_oE 'line continuation in reserved words for in do done'
\
f\
o\
r\
\
 \
i\
 \
i\
n\
\
 1 2
\
d\
o\
\
 echo $i
\
d\
o\
n\
e\

__IN__
1
2
__OUT__

test_oE 'line continuation in reserved words if then elif else fi'
\
i\
f\
\
 false
\
t\
h\
e\
n\
\
 echo 1
e\
l\
i\
f\
 true
\
t\
h\
e\
n\
\
 echo 2
e\
l\
s\
e\
\
 echo 3
\
f\
i\
\

__IN__
2
__OUT__

test_oE 'line continuation in reserved words while do done'
\
w\
h\
i\
l\
e\
\
 true
\
d\
o\
\
 echo 1
break
\
d\
o\
n\
e\
\

__IN__
1
__OUT__

test_oE 'line continuation in reserved words until do done'
\
u\
n\
t\
i\
l\
\
 false
\
d\
o\
\
 echo 1
break
\
d\
o\
n\
e\
\

__IN__
1
__OUT__

test_oE 'line continuation in operators && ||'
true\
&\
&\
\
false\
|\
|\
\
echo 1
__IN__
1
__OUT__

test_oE 'line continuation in function definition'
\
f\
un\
c \
 ( \
 )  \
 # comment
 \
 ( echo ok )

func
__IN__
ok
__OUT__

test_oE 'line continuation in operators <> >> <& >&'
echo 1 >redir
echo 2 \
3\
>\
>\
\
redir \
>\
&\
\
3
cat \
4\
<\
>\
redir \
<\
&\
\
4
__IN__
1
2
__OUT__

test_oE 'line continuation in operator >|' -C
echo XXX >clobber
echo foo \
>\
|\
\
clobber
cat clobber
__IN__
foo
__OUT__

test_oE 'line continuation in here-document operators'
cat \
<\
<\
\
E\
N\
D\

foo
END
cat \
<\
<\
-\
\
E\
O\
F\

		bar
	EOF
__IN__
foo
bar
__OUT__

test_oE 'line continuation in assignment'
fo\
o\
=\
b\
ar
echo $foo
__IN__
bar
__OUT__

test_oE 'line continuation in parameter expansion'
f=foo
# echo $f ${f} ${#f} ${f#f} ${f:+x}
echo \
\
$\
\
f $\
\
{\
\
f\
\
} $\
{\
\
#\
f\
} $\
{\
f\
\
#\
\
f\
\
} $\
{\
f\
\
:\
\
+\
\
x\
\
}
__IN__
foo foo 3 oo x
__OUT__

test_oE 'line continuation in arithmetic expansion'
echo \
$\
\
(\
\
(\
\
1\
\
 \
 + \
 \
2\
\
)\
\
)
__IN__
3
__OUT__

test_oE 'line continuation in command substitution $(...)'
echo \
$\
\
(\
\
echo 1\
\
)
__IN__
1
__OUT__

test_oE 'line continuation in command substitution `...`'
echo \
`\
\
echo 1\
\
`
__IN__
1
__OUT__

test_oE 'single quotes'
bracket 'abc' '"a"' 'a\\b' 'a''''''b'
bracket 'a
b' 'a

b'
__IN__
[abc]["a"][a\\b][ab]
[a
b][a

b]
__OUT__

test_oE 'dollar-single-quotes'
bracket $'' $'a' $'a
b' -$'\"\'\'"\\\a\b\e\f\n\r\t\v\x20\1000'- =$'\x9'$'\11\7'=
bracket $'\cA\ca\c^\c\\\c?' $'\\
'
__IN__
[][a][a
b][-"''"\
	 @0-][=		=]
[][\
]
__OUT__

test_oE 'double quotes'
bracket "abc" "'a'"
bracket "a
b" "a

b"
__IN__
[abc]['a']
[a
b][a

b]
__OUT__

test_oE 'expansions in double quotes'
a=variable
bracket "$a" "${a}" "$(echo command)" "`echo command`" "$((1+10))"
__IN__
[variable][variable][command][command][11]
__OUT__

test_oE 'double quotes in command substitution in double quotes'
bracket "$(bracket "foo
echo ")"
__IN__
[[foo
echo ]]
__OUT__

test_oE 'aliases are ignored in command substitution in double quotes'
alias echo=')'
f() { bracket "$(echo x)"; }
unalias echo
f
__IN__
[x]
__OUT__

test_oE 'backslashes in double quotes'
bracket "a\\b" "a\\\\b"
bracket "a\$b" "a\`b\`c" "a\"b\"c"
bracket "a\
b\
c"
bracket "\ \!\#\$x\%\&\'\(\)\*\+\,\-\.\/"
bracket "\0\1\2\3\4\5\6\7\8\9\:\;\<\=\>\?"
bracket "\@\A\B\C\D\E\F\G\H\I\J\K\L\M\N\O\P\Q\R\S\T\U\V\W\X\Y\Z\[\\\]\^\_"
bracket "\`\a\b\c\d\e\f\g\h\i\j\k\l\m\n\o\p\q\r\s\t\u\v\w\x\y\z\{\|\}\~\`"
bracket "a	
	b"
__IN__
[a\b][a\\b]
[a$b][a`b`c][a"b"c]
[abc]
[\ \!\#$x\%\&\'\(\)\*\+\,\-\.\/]
[\0\1\2\3\4\5\6\7\8\9\:\;\<\=\>\?]
[\@\A\B\C\D\E\F\G\H\I\J\K\L\M\N\O\P\Q\R\S\T\U\V\W\X\Y\Z\[\\]\^\_]
[`\a\b\c\d\e\f\g\h\i\j\k\l\m\n\o\p\q\r\s\t\u\v\w\x\y\z\{\|\}\~`]
[a	
	b]
__OUT__

test_oE 'backslashes in substitution of expansion ${a+b}'
a=a
bracket ${a+\ \!\$x\%\&\(\)\*\+\,\-\.\/ \# \"x\" \'x\'}
bracket ${a+\0\1\2\3\4\5\6\7\8\9\:\;\<\=\>\? \\ \\\\}
bracket ${a+\@\A\B\C\D\E\F\G\H\I\J\K\L\M\N\O\P\Q\R\S\T\U\V\W\X\Y\Z\[\]\^\_}
bracket ${a+\a\b\c\d\e\f\g\h\i\j\k\l\m\n\o\p\q\r\s\t\u\v\w\x\y\z\{\|\}\~ \`\`}
__IN__
[ !$x%&()*+,-./][#]["x"]['x']
[0123456789:;<=>?][\][\\]
[@ABCDEFGHIJKLMNOPQRSTUVWXYZ[]^_]
[abcdefghijklmnopqrstuvwxyz{|}~][``]
__OUT__

test_oE 'backslashes in substitution of expansion ${a+b} in double quotes'
a=a
bracket "${a+\ \!\$x\%\&\(\)\*\+\,\-\.\/ \# \"x\" \'x\'}"
bracket "${a+\0\1\2\3\4\5\6\7\8\9\:\;\<\=\>\? \\ \\\\}"
bracket "${a+\@\A\B\C\D\E\F\G\H\I\J\K\L\M\N\O\P\Q\R\S\T\U\V\W\X\Y\Z\[\]\^\_}"
bracket "${a+\a\b\c\d\e\f\g\h\i\j\k\l\m\n\o\p\q\r\s\t\u\v\w\x\y\z\{\|\}\~ \`\`}"
__IN__
[\ \!$x\%\&\(\)\*\+\,\-\.\/ \# "x" \'x\']
[\0\1\2\3\4\5\6\7\8\9\:\;\<\=\>\? \ \\]
[\@\A\B\C\D\E\F\G\H\I\J\K\L\M\N\O\P\Q\R\S\T\U\V\W\X\Y\Z\[\]\^\_]
[\a\b\c\d\e\f\g\h\i\j\k\l\m\n\o\p\q\r\s\t\u\v\w\x\y\z\{\|}\~ ``]
__OUT__

test_oE 'backslashes in substitution of expansion ${a-b}'
bracket ${u-\ \!\$x\%\&\(\)\*\+\,\-\.\/ \# \"x\" \'x\'}
bracket ${u-\0\1\2\3\4\5\6\7\8\9\:\;\<\=\>\? \\ \\\\}
bracket ${u-\@\A\B\C\D\E\F\G\H\I\J\K\L\M\N\O\P\Q\R\S\T\U\V\W\X\Y\Z\[\]\^\_}
bracket ${u-\a\b\c\d\e\f\g\h\i\j\k\l\m\n\o\p\q\r\s\t\u\v\w\x\y\z\{\|\}\~ \`\`}
__IN__
[ !$x%&()*+,-./][#]["x"]['x']
[0123456789:;<=>?][\][\\]
[@ABCDEFGHIJKLMNOPQRSTUVWXYZ[]^_]
[abcdefghijklmnopqrstuvwxyz{|}~][``]
__OUT__

test_oE 'backslashes in substitution of expansion ${a-b} in double quotes'
bracket "${u-\ \!\$x\%\&\(\)\*\+\,\-\.\/ \# \"x\" \'x\'}"
bracket "${u-\0\1\2\3\4\5\6\7\8\9\:\;\<\=\>\? \\ \\\\}"
bracket "${u-\@\A\B\C\D\E\F\G\H\I\J\K\L\M\N\O\P\Q\R\S\T\U\V\W\X\Y\Z\[\]\^\_}"
bracket "${u-\a\b\c\d\e\f\g\h\i\j\k\l\m\n\o\p\q\r\s\t\u\v\w\x\y\z\{\|\}\~ \`\`}"
__IN__
[\ \!$x\%\&\(\)\*\+\,\-\.\/ \# "x" \'x\']
[\0\1\2\3\4\5\6\7\8\9\:\;\<\=\>\? \ \\]
[\@\A\B\C\D\E\F\G\H\I\J\K\L\M\N\O\P\Q\R\S\T\U\V\W\X\Y\Z\[\]\^\_]
[\a\b\c\d\e\f\g\h\i\j\k\l\m\n\o\p\q\r\s\t\u\v\w\x\y\z\{\|}\~ ``]
__OUT__

# Quote removal is performed before assignment, so the resultant expansions are
# subject to field splitting.
test_oE 'quotes in substitution of expansion ${a=b}'
bracket ${a=\ \!\$x\%\&\(\)\*\+\,\-\.\/ \# \"x\" \'x\'}
bracket ${b=\0\1\2\3\4\5\6\7\8\9\:\;\<\=\>\? \\ \\\\}
bracket ${c=\@\A\B\C\D\E\F\G\H\I\J\K\L\M\N\O\P\Q\R\S\T\U\V\W\X\Y\Z\[\]\^\_}
bracket ${d=\a\b\c\d\e\f\g\h\i\j\k\l\m\n\o\p\q\r\s\t\u\v\w\x\y\z\{\|\}\~ \`\`}
bracket $a
bracket $b
bracket $c
bracket $d
__IN__
[!$x%&()*+,-./][#]["x"]['x']
[0123456789:;<=>?][\][\\]
[@ABCDEFGHIJKLMNOPQRSTUVWXYZ[]^_]
[abcdefghijklmnopqrstuvwxyz{|}~][``]
[!$x%&()*+,-./][#]["x"]['x']
[0123456789:;<=>?][\][\\]
[@ABCDEFGHIJKLMNOPQRSTUVWXYZ[]^_]
[abcdefghijklmnopqrstuvwxyz{|}~][``]
__OUT__

# Quote removal is performed before assignment, but the resultant expansions
# are not subject to field splitting because they are double-quoted.
test_oE 'quotes in substitution of expansion ${a=b} in double quotes'
bracket "${a=\ \!\$x\%\&\(\)\*\+\,\-\.\/ \# \"x\" \'x\'}"
bracket "${b=\0\1\2\3\4\5\6\7\8\9\:\;\<\=\>\? \\ \\\\}"
bracket "${c=\@\A\B\C\D\E\F\G\H\I\J\K\L\M\N\O\P\Q\R\S\T\U\V\W\X\Y\Z\[\]\^\_}"
bracket "${d=\a\b\c\d\e\f\g\h\i\j\k\l\m\n\o\p\q\r\s\t\u\v\w\x\y\z\{\|\}\~ \`\`}"
bracket "${e=a"b"c}" "${f=a"*"c}" "${g=a"\"\""c}" "${h=a"\\"c}" "${i=a"''"c}"
bracket "${j=a'b'c}" "${k=a'*'c}" "${l=a'""'c}"   "${m=a'\'c}"
bracket "$a"
bracket "$b"
bracket "$c"
bracket "$d"
bracket "$e" "$f" "$g" "$h" "$i"
bracket "$j" "$k" "$l" "$m"
__IN__
[\ \!$x\%\&\(\)\*\+\,\-\.\/ \# "x" \'x\']
[\0\1\2\3\4\5\6\7\8\9\:\;\<\=\>\? \ \\]
[\@\A\B\C\D\E\F\G\H\I\J\K\L\M\N\O\P\Q\R\S\T\U\V\W\X\Y\Z\[\]\^\_]
[\a\b\c\d\e\f\g\h\i\j\k\l\m\n\o\p\q\r\s\t\u\v\w\x\y\z\{\|}\~ ``]
[abc][a*c][a""c][a\c][a''c]
[a'b'c][a'*'c][a''c][a'\'c]
[\ \!$x\%\&\(\)\*\+\,\-\.\/ \# "x" \'x\']
[\0\1\2\3\4\5\6\7\8\9\:\;\<\=\>\? \ \\]
[\@\A\B\C\D\E\F\G\H\I\J\K\L\M\N\O\P\Q\R\S\T\U\V\W\X\Y\Z\[\]\^\_]
[\a\b\c\d\e\f\g\h\i\j\k\l\m\n\o\p\q\r\s\t\u\v\w\x\y\z\{\|}\~ ``]
[abc][a*c][a""c][a\c][a''c]
[a'b'c][a'*'c][a''c][a'\'c]
__OUT__

# See quote-y.tst
#test_oE 'backslashes in substitution of expansion ${a?b}'
#__IN__
#__OUT__

test_oE 'single and double quotes in substitution of expansion ${a+b}'
a=a
bracket ${a+a"b"c} ${a+a"*"c} ${a+a"\"\""c} ${a+a"\\"c} ${a+a"''"c}
bracket ${a+a'b'c} ${a+a'*'c} ${a+a'""'c}   ${a+a'\'c}
__IN__
[abc][a*c][a""c][a\c][a''c]
[abc][a*c][a""c][a\c]
__OUT__

test_oE 'single quotes in substitution of expansion ${a+b} in double quotes'
a=a
unset b
bracket "${a+a'$b'c}"
bracket "${a+a'b}"
bracket "${a+a$'\t'b}"
__IN__
[a''c]
[a'b]
[a$'\t'b]
__OUT__
#}"

test_oE 'tilde in substitution of expansion ${a+b}'
HOME=/home a=a
bracket ${a+~} "${a+~}"
__IN__
[/home][~]
__OUT__

test_oE 'single and double quotes in substitution of expansion ${a-b}'
bracket ${u-a"b"c} ${u-a"*"c} ${u-a"\"\""c} ${u-a"\\"c} ${u-a"''"c}
bracket ${u-a'b'c} ${u-a'*'c} ${u-a'""'c}   ${u-a'\'c}
__IN__
[abc][a*c][a""c][a\c][a''c]
[abc][a*c][a""c][a\c]
__OUT__

test_oE 'single quotes in substitution of expansion ${a-b} in double quotes'
unset a b
bracket "${a-a'$b'c}"
bracket "${a-a'b}"
bracket "${a-a$'\t'b}"
__IN__
[a''c]
[a'b]
[a$'\t'b]
__OUT__
#}"

test_oE 'tilde in substitution of expansion ${a-b}'
HOME=/home
bracket ${a-~} "${a-~}"
__IN__
[/home][~]
__OUT__

test_oE 'single and double quotes in substitution of expansion ${a=b}'
bracket ${a=a"b"c} ${b=a"*"c} ${c=a"\"\""c} ${d=a"\\"c} ${e=a"''"c}
bracket ${f=a'b'c} ${g=a'*'c} ${h=a'""'c}   ${i=a'\'c}
bracket $a $b $c $d $e
bracket $f $g $h $i
__IN__
[abc][a*c][a""c][a\c][a''c]
[abc][a*c][a""c][a\c]
[abc][a*c][a""c][a\c][a''c]
[abc][a*c][a""c][a\c]
__OUT__

test_oE 'single quotes in substitution of expansion ${a=b} in double quotes'
unset a b c d
bracket "${a=a'$b'c}"
bracket "${c=a'b}"
bracket "${d=a$'\t'b}"
bracket "$a" "$c" "$d"
__IN__
[a''c]
[a'b]
[a$'\t'b]
[a''c][a'b][a$'\t'b]
__OUT__
#'
#}"

test_oE 'tilde in substitution of expansion ${a=b}'
HOME=/home
bracket ${a=~} "${b=~}"
bracket "$a" "$b"
__IN__
[/home][~]
[/home][~]
__OUT__

# See quote-y.tst
#test_oE 'single and double quotes in substitution of expansion ${a?b}'
#__IN__
#__OUT__

test_oE 'quotes in pattern of expansions'
# double quotes
a='*""ok'
bracket ${a#"*"\"\"} "${a#"*"\"\"}"
# single quotes
b="*''ok"
bracket ${b#'*'\'\'} "${b#'*'\'\'}"
# backslashes
c='*\ok'
bracket ${c#\*\\} "${c#\*\\}"
__IN__
[ok][ok]
[ok][ok]
[ok][ok]
__OUT__

test_oE 'backslashes resulting from expansions (not a pattern)'
# The backslashes are not subject to quote removal since they were not present
# in the original word before parameter expansion.
v='\a\b\c'
bracket "$v"
bracket $v
__IN__
[\a\b\c]
[\a\b\c]
__OUT__

(
(> '\' > '\*') 2>/dev/null || skip="true"

test_oE 'backslashes resulting from expansions (a pattern)'
# This backslash escapes the asterisk, so pathname expansion does not match
# with '\' or '\*'.
v='\*'
bracket "$v"
bracket $v
__IN__
[\*]
[\*]
__OUT__

)

test_oE 'quoted words are not reserved words'
echo echo if command >if
chmod a+x if
PATH=.:$PATH
\if
"i"f
i'f'
__IN__
if command
if command
if command
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
