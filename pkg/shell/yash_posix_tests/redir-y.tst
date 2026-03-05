# redir-y.tst: yash-specific test of redirections

exec 3>&- 4>&- 5>&- 6>&- 7>&- 8>&- 9>&-

echo in0 >in0
echo in1 >in1
echo 'in*' >'in*'

echo PS1= >yashrc

# Many existing shells return an error in this case, although this does not seem
# strictly POSIX-conforming.
test_O -d -e n 'overwrite redirection, -C, symlink to non-existing file' -C
ln -s _no_such_file_ badsymlink
echo foo >badsymlink
__IN__

test_oE -e 0 \
    'pathname expansion in redirection operand (interactive), success'\
    -i +m --rcfile=yashrc
cat <i*0
cat <"i"'n'\*
HOME='in*'
cat <~ # result of tilde expansion is not subject to pathname expansion
__IN__
in0
in*
in*
__OUT__

test_O -d -e 2 \
    'pathname expansion in redirection operand (interactive), multi-match' \
    -i +m --rcfile=yashrc
cat <in* # more than one matching pathname
__IN__

test_O -d -e 2 \
    'pathname expansion in redirection operand (interactive), no match, non-POSIX' \
    -i +m --rcfile=yashrc
mkdir globbing1
# pathname expansion does not match any file, it is an error
echo foo >globbing1/no_such_file* &&
echo not reached
__IN__

(
posix="true"
export ENV=yashrc

test_oE -e 0 'pathname expansion in redirection operand (interactive), no match, POSIX' \
    -i +m
mkdir globbing2
# pathname expansion does not match any file, so the filename operand is used
# intact
echo foo >globbing2/no_such_file*
cat <'globbing2/no_such_file*'
__IN__
foo
__OUT__

)

test_o -e 0 'redirection to special built-in affects current environment'
unset x
: < ${x=/dev/null}
echo $x
__IN__
/dev/null
__OUT__

test_o -e 0 'redirection to external command affects current environment'
unset x
"$TESTEE" -c : < ${x=/dev/null}
echo $x
__IN__
/dev/null
__OUT__

test_o -e 0 'redirection to grouping affects current environment'
unset x
{ :; } < ${x=/dev/null}
echo $x
__IN__
/dev/null
__OUT__

test_o -e 0 'redirection to subshell affects current environment'
unset x
(:) < ${x=/dev/null}
echo $x
__IN__
/dev/null
__OUT__

test_OE -e 0 'input duplication of unwritable file descriptor'
3>/dev/null <&3
__IN__

test_OE -e 0 'output duplication of unreadable file descriptor'
3</dev/null >&3
__IN__

test_oE -e 0 'tilde expansion not performed in here-document operand'
HOME=/home
cat <<~
/home
~
__IN__
/home
__OUT__

test_oE -e 0 'parameter expansion not performed in here-document operand'
foo=FOO
cat <<$foo
FOO
$foo
__IN__
FOO
__OUT__

test_oE -e 0 'command substitution not performed in here-document operand'
cat <<$(echo foo)`echo bar`
foobar
$(echo foo)`echo bar`
__IN__
foobar
__OUT__

test_oE -e 0 'arithmetic expansion not performed in here-document operand'
cat <<$((1+1))
foo
$((1+1))
__IN__
foo
__OUT__

test_oE -e 0 'complex expansion in here-document' -s 1 '"  "' 3
IFS=-
printf '[%s]' ${1+"$@"}; echo
printf '[%s]' "${1+"$@"}"; echo
cat <<END
[$*]
[$@]
[${1+"$*"}]
[${1+"$@"}]
END

echo =====

IFS==
cat <<END
[$*]
[$@]
[${1+"$*"}]
[${1+"$@"}]
END
__IN__
[1]["  "][3]
[1]["  "][3]
[1-"  "-3]
[1-"  "-3]
[1-"  "-3]
[1-"  "-3]
=====
[1="  "=3]
[1="  "=3]
[1="  "=3]
[1="  "=3]
__OUT__

test_oE -e 0 'complex expansion with backslashes in here-document' -s 1 '\' 3
IFS='\'
cat <<END
[$*]
[$@]
[${1+"$*"}]
[${1+"$@"}]
END
__IN__
[1\\\3]
[1\\\3]
[1\\\3]
[1\\\3]
__OUT__

test_oE -e 0 'line-continued end-of-here-document indicator (unquoted)' -e
cat <<echo
ec\
ho
echo
__IN__
echo
__OUT__
# See also the test 'no quote removal with quoted here-document delimiter'
# in redir-p.tst

# This is not required by POSIX, but many other shells behave this way.
test_oE -e 0 'end-of-here-document indicator just before EOF (unquoted)' \
    -c 'cat <<END
foo
END'
__IN__
foo
__OUT__

test_oE -e 0 'end-of-here-document indicator just before EOF (quoted)' \
    -c 'cat <<\END
foo
END'
__IN__
foo
__OUT__

test_oE -e 0 'here-document is expanded in current shell' -e
unset a
cat <<END
${a=foo}
END
echo $a
__IN__
foo
foo
__OUT__

test_oE -e 0 'duplicating input to the same file descriptor'
echo foo | cat <&0
__IN__
foo
__OUT__

test_oE -e 0 'duplicating output to the same file descriptor'
echo foo >&1
__IN__
foo
__OUT__

# Test various file descriptor combinations to ensure the shell correctly
# "dup"s the pipe file descriptors after opening a pipe.
test_oE 'pipe redirection'
exec 4>>|3; echo 4-3 >&4; exec 4>&-; cat <&3; exec 3<&-
exec 4>>|6; echo 4-6 >&4; exec 4>&-; cat <&6; exec 6<&-
exec 5>>|3; echo 5-3 >&5; exec 5>&-; cat <&3; exec 3<&-
exec 5>>|6; echo 5-6 >&5; exec 5>&-; cat <&6; exec 6<&-
exec 5>>|4; echo 5-4 >&5; exec 5>&-; cat <&4; exec 4<&-
exec 3>>|6; echo 3-6 >&3; exec 3>&-; cat <&6; exec 6<&-
exec 3>>|4; echo 3-4 >&3; exec 3>&-; cat <&4; exec 4<&-
exec 9>&1
exec  >>|0; echo 1-0    ; exec  >&9; cat <&0;
__IN__
4-3
4-6
5-3
5-6
5-4
3-6
3-4
1-0
__OUT__

test_oE 'using pipe redirection in subshell'
exec 3>&1
{
    while read -r i && [ "$i" -lt 5 ]; do
        echo "$i" >&3
        echo "$((i+1))"
    done | {
        echo 0
        cat -u
    }
} >>|0
__IN__
0
1
2
3
4
__OUT__

test_oE -e 0 'input process redirection' -e
cat <(echo foo)
cat <(echo bar)-
__IN__
foo
bar
__OUT__

test_oE -e 0 'output process redirection' -e
echo >(read i && echo $((i+1)))99 | cat
# "cat" ensures the output is flushed before the shell exits
__IN__
100
__OUT__

test_oE -e 0 'process redirection is run in subshell' -e
i=0
echo >(read i && echo $((i+1))) 99 | cat
# "cat" ensures the output is flushed before reaching this line
echo $i
__IN__
100
0
__OUT__

test_x -e 0 'exit status of process redirection is ignored'
<(false) >(false)
__IN__

test_oE -e 0 'empty here-string'
cat <<<""
__IN__

__OUT__

test_oE -e 0 'tilde expansion in here-string'
HOME=/home
cat <<<~/foo
__IN__
/home/foo
__OUT__

test_oE -e 0 'double quotation and parameter expansion in here-string'
foo=FOOOO
cat <<<"${foo%OO}"
__IN__
FOO
__OUT__

test_oE -e 0 'multi-line here-string'
cat <<<'1
2'
__IN__
1
2
__OUT__

test_oE -e 0 'space after here-string operator'
cat <<< foo
__IN__
foo
__OUT__

test_oE -e 0 'here-string with non-default file descriptor'
cat 3<<<foo <&3
__IN__
foo
__OUT__

test_OE -e 0 'IO_NUMBER can be redirection operand'
> 1> 2< 2>>3 <<4
4
__IN__

(
posix="true"

test_Oe -e 2 'IO_NUMBER as redirection operand (filename)'
# Here the token "12" is an IO_NUMBER token for the second redirection, and
# hence cannot be the operand of the first.
> 12> 34
__IN__
syntax error: put a space between `2' and `>' for disambiguation
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'IO_NUMBER as redirection operand (here-document)'
# Here the token "01" is an IO_NUMBER token for the second redirection, and
# hence cannot be the operand of the first.
<< 01<< 23
01
23
__IN__
syntax error: put a space between `1' and `<' for disambiguation
syntax error: here-document content for <<01 is missing
syntax error: here-document content for <<23 is missing
__ERR__
#'
#`
#'
#`

test_Oe -e 2 'IO_LOCATION as redirection operand (filename)'
# Here the token "{n}" is an IO_LOCATION token for the second redirection, and
# hence cannot be the operand of the first.
> {n}> foo
__IN__
syntax error: put a space between `}' and `>' for disambiguation
__ERR__
#'
#`
#'
#`
)

test_Oe -e 2 'IO_LOCATION not yet supported'
{n}> foo
__IN__
syntax error: specifying file descriptor in braces is not supported
__ERR__

test_Oe -e 2 'missing target for <'
<
__IN__
syntax error: the redirection target is missing
__ERR__

test_Oe -e 2 'missing target for <>'
<>
__IN__
syntax error: the redirection target is missing
__ERR__

test_Oe -e 2 'missing target for <& (EOF)'
<&
__IN__
syntax error: the redirection target is missing
__ERR__

test_Oe -e 2 'missing target for <& (line continuation and comment)'
<&\
    #
__IN__
syntax error: the redirection target is missing
__ERR__

test_Oe -e 2 'missing target for >'
>
__IN__
syntax error: the redirection target is missing
__ERR__

test_Oe -e 2 'missing target for >|'
>|
__IN__
syntax error: the redirection target is missing
__ERR__

test_Oe -e 2 'missing target for >&'
>&
__IN__
syntax error: the redirection target is missing
__ERR__

test_Oe -e 2 'missing target for >>'
>>
__IN__
syntax error: the redirection target is missing
__ERR__

test_Oe -e 2 'missing target for >>|'
>>|
__IN__
syntax error: the redirection target is missing
__ERR__

test_Oe -e 2 'missing target for <<<'
<<<
__IN__
syntax error: the redirection target is missing
__ERR__

test_Oe -e 2 'missing delimiter after <<'
<<
__IN__
syntax error: the end-of-here-document indicator is missing
__ERR__

test_Oe -e 2 'missing delimiter after <<-'
<<-
__IN__
syntax error: the end-of-here-document indicator is missing
__ERR__

test_Oe -e 2 'newline in end-of-here-document indicator'
<<'
'
__IN__
syntax error: the end-of-here-document indicator contains a newline
__ERR__

test_Oe -e 2 'unclosed here-document (unquoted)'
cat <<END
foo
__IN__
syntax error: the here-document content is not closed by `END'
__ERR__
#'
#`
: <<END
END

test_Oe -e 2 'unclosed single quote in here-document delimiter'
cat <<`'`
``
`'`
__IN__
syntax error: the end-of-here-document indicator is not properly quoted
__ERR__

test_Oe -e 2 'unclosed double quote in here-document delimiter'
cat <<`"`
``
`"`
__IN__
syntax error: the end-of-here-document indicator is not properly quoted
__ERR__

test_Oe -e 2 'missing newline and here-document delimiter (unquoted)' \
    -c 'cat <<END'
__IN__
yash -c:1: syntax error: here-document content for <<END is missing
__ERR__
: <<END
END

test_Oe -e 2 'unclosed here-document (quoted)'
cat <<\END
foo
__IN__
syntax error: the here-document content is not closed by `END'
__ERR__
: <<END
END

test_Oe -e 2 'missing newline and here-document delimiter (quoted)' \
    -c 'cat <<\END'
__IN__
yash -c:1: syntax error: here-document content for <<\END is missing
__ERR__
: <<END
END

test_O -d -e 2 'space between < and ( in process redirection'
< (:)
__IN__

test_Oe -e 2 'unclosed input process redirection'
echo not printed <(
__IN__
syntax error: unclosed process redirection
__ERR__

test_Oe -e 2 'unclosed output process redirection'
echo not printed >(
__IN__
syntax error: unclosed process redirection
__ERR__

(
posix="true"

test_Oe -e 2 'no process redirection <() in POSIX mode'
<()
__IN__
syntax error: process redirection is not supported in the POSIXly-correct mode
__ERR__

test_Oe -e 2 'no process redirection >() in POSIX mode'
>()
__IN__
syntax error: process redirection is not supported in the POSIXly-correct mode
__ERR__

test_Oe -e 2 'no pipe redirection in POSIX mode'
>>|0
__IN__
syntax error: pipe redirection is not supported in the POSIXly-correct mode
__ERR__

test_Oe -e 2 'no here-string in POSIX mode'
<<<foo
__IN__
syntax error: here-string is not supported in the POSIXly-correct mode
__ERR__

test_Oe -e 2 'keyword after redirection in compound command (POSIX)'
{ { echo not printed; } >/dev/null }
__IN__
syntax error: unexpected word after redirection
syntax error: (maybe you missed `;'?)
__ERR__
#'
#`
#)

test_Oe -e 2 'keyword after redirection on subshell (POSIX)'
{ (echo not printed) >/dev/null }
__IN__
syntax error: unexpected word after redirection
syntax error: (maybe you missed `;'?)
__ERR__
#'
#`
#)

# Some relevant tests in grouping-y.tst.

)

test_OE -e 0 'keyword after redirection in compound command (non-POSIX)'
{ { echo not printed; } >/dev/null }
{ ( echo not printed  ) >/dev/null }
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
