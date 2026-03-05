# tilde-p.tst: test of tilde expansion for any POSIX-compliant shell

posix="true"
setup -d

(
setup 'HOME=/foo/bar'

test_oE 'quoted tilde is not expanded (in command word)'
bracket \~ '~' "~" ~\/
__IN__
[~][~][~][~/]
__OUT__

test_oE 'unnamed tilde expansion, end of word'
bracket ~
HOME=/tilde/expansion
bracket ~
__IN__
[/foo/bar]
[/tilde/expansion]
__OUT__

test_oE 'unnamed tilde expansion, followed by slash'
bracket ~/ ~/baz
HOME=/tilde/expansion
bracket ~/ ~/slash
__IN__
[/foo/bar/][/foo/bar/baz]
[/tilde/expansion/][/tilde/expansion/slash]
__OUT__

test_OE -e 0 'exit status of successful unnamed tilde expansion (in command word)'
: ~ ~/ ~/foo
__IN__

test_oE 'quoted tilde is not expanded in assignment'
a=\~ b='~' c="~" d=~\/
bracket "$a" "$b" "$c" "$d"
__IN__
[~][~][~][~/]
__OUT__

test_oE 'unnamed tilde expansion in assignment, end of word'
a=~
HOME=/tilde/expansion
b=~
bracket "$a" "$b"
__IN__
[/foo/bar][/tilde/expansion]
__OUT__

test_oE 'unnamed tilde expansion in assignment, followed by slash'
a=~/ b=~/baz
HOME=/tilde/expansion
c=~/ d=~/slash
bracket "$a" "$b" "$c" "$d"
__IN__
[/foo/bar/][/foo/bar/baz][/tilde/expansion/][/tilde/expansion/slash]
__OUT__

test_oE 'unnamed tilde expansion in assignment, followed by colon'
a=~: b=~:baz
HOME=/tilde/expansion
c=~: d=~:colon
bracket "$a" "$b" "$c" "$d"
__IN__
[/foo/bar:][/foo/bar:baz][/tilde/expansion:][/tilde/expansion:colon]
__OUT__

test_oE -e 0 'unnamed tilde expansion in assignment, following colon'
a=:~ b=baz:~
HOME=/tilde/expansion
c=:~ d=colon:~
bracket "$a" "$b" "$c" "$d"
__IN__
[:/foo/bar][baz:/foo/bar][:/tilde/expansion][colon:/tilde/expansion]
__OUT__

test_oE -e 0 'unnamed tilde expansion in assignment, between colon'
a=:~: b=baz:~:baz
HOME=/tilde/expansion
c=:~: d=colon:~:colon
bracket "$a" "$b" "$c" "$d"
__IN__
[:/foo/bar:][baz:/foo/bar:baz][:/tilde/expansion:][colon:/tilde/expansion:colon]
__OUT__

test_oE -e 0 'many unnamed tilde expansions in assignment'
a=~:x:~/y:~:~
bracket "$a"
__IN__
[/foo/bar:x:/foo/bar/y:/foo/bar:/foo/bar]
__OUT__

test_OE -e 0 'exit status of successful unnamed tilde expansion in assignment'
a=~:x:~/y:~:~
__IN__

)

test_oE -e 0 'empty HOME'
HOME=
bracket ~
__IN__
[]
__OUT__

test_oE -e 0 'HOME with trailing slash'
HOME=/foo/bar/
bracket ~ ~/~
__IN__
[/foo/bar/][/foo/bar/~]
__OUT__

test_oE -e 0 'HOME=/'
HOME=/
bracket ~ ~/foo
__IN__
[/][/foo]
__OUT__

test_oE -e 0 'HOME=//'
HOME=//
bracket ~ ~/foo
__IN__
[//][//foo]
__OUT__

(
if
    logname=$(logname)
    if [ "$logname" ]; then LOGNAME=$logname; fi
    unset logname
    ! { [ "${LOGNAME-}" ] && export LOGNAME; }
then
    skip="true"
elif
    # The current user's name has to be portable.
    printf '%s\n' "$LOGNAME" | \
        grep -q '[^0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz._-]'
then
    skip="true"
elif
    # Tests of named tilde expansions depend on the home directory of the
    # current user.
    ! { HOME="$(eval printf \'%s\\\n\' \~$LOGNAME)" &&
        export HOME &&
        [ "$HOME" ]; }
then
    skip="true"
fi

if "${skip:-false}"; then
    LOGNAME= HOME=
fi

testcase "$LINENO" 'tilde with quoted name is not expanded (in command word)' \
    3<<__IN__ 4<<__OUT__ 5</dev/null
bracket ~\\$LOGNAME ~"$LOGNAME" ~'$LOGNAME' ~$LOGNAME\\/
__IN__
[~$LOGNAME][~$LOGNAME][~$LOGNAME][~$LOGNAME/]
__OUT__

testcase "$LINENO" 'named tilde expansion, end of word' \
    3<<__IN__ 4<<__OUT__ 5</dev/null
bracket ~$LOGNAME
__IN__
[$HOME]
__OUT__

testcase "$LINENO" 'named tilde expansion, followed by slash' \
    3<<__IN__ 4<<__OUT__ 5</dev/null
bracket ~$LOGNAME/ ~$LOGNAME/foo
__IN__
[$HOME/][$HOME/foo]
__OUT__

testcase "$LINENO" -e 0 \
    'exit status of successful named tilde expansion (in command word)' \
    3<<__IN__ 4</dev/null 5</dev/null
: ~$LOGNAME ~$LOGNAME/ ~$LOGNAME/foo
__IN__

testcase "$LINENO" 'tilde with quoted name is not expanded in assignment' \
    3<<__IN__ 4<<__OUT__ 5</dev/null
a=~\\$LOGNAME b=~"$LOGNAME" c=~'$LOGNAME' d=~$LOGNAME\\/
bracket "\$a" "\$b" "\$c" "\$d"
__IN__
[~$LOGNAME][~$LOGNAME][~$LOGNAME][~$LOGNAME/]
__OUT__

testcase "$LINENO" 'named tilde expansion in assignment, end of word' \
    3<<__IN__ 4<<__OUT__ 5</dev/null
a=~$LOGNAME
bracket "\$a"
__IN__
[$HOME]
__OUT__

testcase "$LINENO" 'named tilde expansion in assignment, followed by slash' \
    3<<__IN__ 4<<__OUT__ 5</dev/null
a=~$LOGNAME/ b=~$LOGNAME/foo
bracket "\$a" "\$b"
__IN__
[$HOME/][$HOME/foo]
__OUT__

testcase "$LINENO" 'named tilde expansion in assignment, followed by colon' \
    3<<__IN__ 4<<__OUT__ 5</dev/null
a=~$LOGNAME: b=~$LOGNAME:foo
bracket "\$a" "\$b"
__IN__
[$HOME:][$HOME:foo]
__OUT__

testcase "$LINENO" 'named tilde expansion in assignment, following colon' \
    3<<__IN__ 4<<__OUT__ 5</dev/null
a=:~$LOGNAME b=foo:~$LOGNAME
bracket "\$a" "\$b"
__IN__
[:$HOME][foo:$HOME]
__OUT__

testcase "$LINENO" 'named tilde expansion in assignment, between colon' \
    3<<__IN__ 4<<__OUT__ 5</dev/null
a=:~$LOGNAME: b=foo:~$LOGNAME:bar
bracket "\$a" "\$b"
__IN__
[:$HOME:][foo:$HOME:bar]
__OUT__

testcase "$LINENO" 'many named tilde expansions in assignment' \
    3<<__IN__ 4<<__OUT__ 5</dev/null
a=~$LOGNAME:x:~$LOGNAME/y:~$LOGNAME:~$LOGNAME
bracket "\$a"
__IN__
[$HOME:x:$HOME/y:$HOME:$HOME]
__OUT__

testcase "$LINENO" -e 0 \
    'exit status of successful named tilde expansion in assignment' \
    3<<__IN__ 4</dev/null 5</dev/null
a=~$LOGNAME:x:~$LOGNAME/y:~$LOGNAME:~$LOGNAME
__IN__

)

test_oE 'result of tilde expansion is not subject to field splitting'
HOME='/path/with  space'
bracket ~
__IN__
[/path/with  space]
__OUT__

test_oE 'result of tilde expansion is not subject to parameter expansion'
HOME='$x' x='X'
bracket ~
__IN__
[$x]
__OUT__

test_oE 'result of tilde expansion is not subject to command substitution'
HOME='$(echo X)`echo Y`'
bracket ~
__IN__
[$(echo X)`echo Y`]
__OUT__

test_oE 'result of tilde expansion is not subject to arithmetic expansion'
HOME='$((1+1))'
bracket ~
__IN__
[$((1+1))]
__OUT__

test_oE 'result of tilde expansion is not subject to pathname expansion'
HOME='*'
bracket ~
__IN__
[*]
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
