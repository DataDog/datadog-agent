# redir-p.tst: test of redirections for any POSIX-compliant shell

posix="true"

exec 3>&- 4>&- 5>&- 6>&- 7>&- 8>&- 9>&-

echo in0 >in0
echo in1 >in1
echo in2 >in2
echo in3 >in3
echo 'in*' >'in*'

test_o 'quoted file descriptor'
echo \2>quotedfd
echo ---
cat quotedfd
__IN__
---
2
__OUT__

test_oE 'file descriptor must immediately precede operator'
echo 1 >precede1
echo 2>precede2
echo ---
cat precede1
echo ----
cat precede2
__IN__

---
1
----
__OUT__

test_o 'file descriptors up to 9 are supported'
cat 9<in0 8<&9 7<&8 6<&7 5<&6 4<&5 3<&4 0<&3
__IN__
in0
__OUT__

test_x -e 0 'multi-digit file descriptor'
echo 11>multidigit # should not be interpreted as "echo 1 1>multidigit"
test "$(cat multidigit)" != 1
__IN__

test_oE -e 0 'tilde expansion in redirection operand'
HOME=$PWD
cat <~/in0
__IN__
in0
__OUT__

test_oE -e 0 'parameter expansion in redirection operand'
i=xxxin0
cat <${i#xxx}
__IN__
in0
__OUT__

test_oE -e 0 'command substitution in redirection operand'
cat <`echo in`$(echo 0)
__IN__
in0
__OUT__

test_oE -e 0 'arithmetic expansion in redirection operand'
cat <in$((1-1))
__IN__
in0
__OUT__

test_oE -e 0 'quote removal in redirection operand'
cat <\i'n'"0"
__IN__
in0
__OUT__

test_oE -e 0 'pathname expansion in redirection operand (non-interactive)'
cat <in*
__IN__
in*
__OUT__

test_O -e n 'redirections apply in order of appearance'
echo - 1>/dev/null 3>&1 2>&3 3>&-
1>&- 2>&1
__IN__

test_O 'redirection without command name runs in subshell'
unset x
< ${x=no/such/file}
<<END
${x=foo}
END
# The assignments in the subshell are not visible from the main shell.
${x+echo not printed}
__IN__

test_oE 'input redirection, success'
cat 0<in0
cat  <in1
__IN__
in0
in1
__OUT__

test_O -d -e n 'input redirection, failure'
<_no_such_dir_/foo
__IN__

test_oE 'overwrite redirection, +C, success'
echo foo  >overwrite1
echo bar 1>overwrite2
          >overwrite2
echo ---
cat overwrite1 overwrite2
__IN__
---
foo
__OUT__

test_oE 'overwrite redirection, -C, success' -C
echo foo >overwrite3 # non-existing file
echo bar >/dev/null  # existing non-regular file
echo ---
cat overwrite3
__IN__
---
foo
__OUT__

test_O -d -e n 'overwrite redirection, -C, existing file error' -C
echo foo >overwrite4
echo boo >overwrite4
__IN__

test_o 'overwrite redirection, -C, existing file not modified' -C
echo foo >overwrite5
echo boo >overwrite5 || :
echo ---
cat overwrite5
__IN__
---
foo
__OUT__

test_O -d -e n 'overwrite redirection, creation failure'
>_no_such_dir_/foo
__IN__

test_oE 'clobbering redirection, +C, success'
echo foo  >|clobber1
echo bar 1>|clobber2
          >|clobber2
echo --- $?
cat overwrite1 overwrite2
__IN__
--- 0
foo
__OUT__

test_oE 'clobbering redirection, -C, success' -C
echo foo >|clobber3  # non-existing file
echo --- $?
echo bar >|/dev/null # existing non-regular file
echo ---- $?
cat clobber3
echo -----
>|clobber3  # existing file
echo ------ $?
cat clobber3
__IN__
--- 0
---- 0
foo
-----
------ 0
__OUT__

test_O -d -e n 'clobbering redirection, creation failure'
>|_no_such_dir_/foo
__IN__

test_oE 'appending redirection, success, new file'
echo foo >>append1
echo --- $?
cat append1
__IN__
--- 0
foo
__OUT__

test_oE 'appending redirection, success, existing file'
echo foo >>append2
echo --- $?
echo bar >>append2
echo ---- $?
cat append2
__IN__
--- 0
---- 0
foo
bar
__OUT__

test_oE 'effect of appending redirection'
{
    echo foo >&3 &&
    echo bar >&4 &&
    echo baz >&3 &&
    echo qux >&4
} 3>>append3 4>>append3 &&
cat append3
__IN__
foo
bar
baz
qux
__OUT__

test_oE -e 0 'in-out redirection, success'
echo foo 1<>inout1
echo --- $?
cat <>inout1
__IN__
--- 0
foo
__OUT__

test_O -d -e n 'in-out redirection, failure'
<>_no_such_dir_/foo
__IN__

test_oE -e 0 'input duplication, success'
cat 3<in3  <&3 &&
cat 3<in3 0<&"$((1+2))"
__IN__
in3
in3
__OUT__

(
setup 'exec 3<&-'

test_O -d -e n 'input duplication, failure (closed file descriptor)'
<&3
__IN__

)

test_O -d -e n 'input duplication, failure (unreadable file descriptor)'
cat 3>/dev/null <&3
__IN__

test_OE -e 0 'input closing, success, open file descriptor'
<&- && 0<&-
__IN__

test_OE -e 0 'input closing, success, closed file descriptor'
<&- <&-
__IN__

test_O -e n 'effect of input closing'
cat <&-
__IN__

test_oE 'output duplication, success'
echo foo 3>dup1  >&3
echo --- $?
echo bar 3>>dup1 1>&"$((1+2))"
echo ---- $?
cat dup1
__IN__
--- 0
---- 0
foo
bar
__OUT__

(
setup 'exec 3>&-'

test_O -d -e n 'output duplication, failure (closed file descriptor)'
>&3
__IN__

)

test_O -d -e n 'output duplication, failure (unreadable file descriptor)'
3</dev/null >&3
__IN__

test_OE -e 0 'output closing, success, open file descriptor'
>&- && 1>&-
__IN__

test_OE -e 0 'output closing, success, closed file descriptor'
>&- >&-
__IN__

test_O -e n 'effect of output closing'
echo >&-
__IN__

test_oE -e 0 'effect of here-document'
cat <<END
here

	document
END
__IN__
here

	document
__OUT__

test_oE -e 0 'here-document with non-default file descriptor'
cat 3<<END <&3
foo
END
__IN__
foo
__OUT__

test_oE -e 0 'no tilde expansion with unquoted here-document delimiter'
HOME=/home
cat <<END
tilde ~
END
__IN__
tilde ~
__OUT__

test_oE -e 0 'parameter expansion with unquoted here-document delimiter'
foo=foooo
cat <<END
parameter ${foo%"oo"}
END
__IN__
parameter foo
__OUT__

test_oE -e 0 'command substitution with unquoted here-document delimiter'
cat <<END
command $(echo "foo") `echo "bar"`
END
__IN__
command foo bar
__OUT__

test_oE -e 0 'arithmetic expansion with unquoted here-document delimiter'
cat <<END
arithmetic $((1+10))
END
__IN__
arithmetic 11
__OUT__

test_oE -e 0 'backslash with unquoted here-document delimiter'
foo=bar
cat <<END
backslash \a \$foo \\\\ \`\` \"\" line-\
continuation
END
__IN__
backslash \a $foo \\ `` \"\" line-continuation
__OUT__

test_oE -e 0 'single and double quotes with unquoted here-document delimiter'
cat <<END
quote 'single' "double \$ 'a' " \$ 'a'
END
__IN__
quote 'single' "double $ 'a' " $ 'a'
__OUT__

test_oE -e 0 'no tilde expansion with quoted here-document delimiter'
HOME=/home
cat <<'END'
tilde ~
END
__IN__
tilde ~
__OUT__

test_oE -e 0 'no parameter expansion with quoted here-document delimiter'
foo=foooo
cat <<'END'
parameter ${foo%"oo"}
END
__IN__
parameter ${foo%"oo"}
__OUT__

test_oE -e 0 'no command substitution with quoted here-document delimiter'
cat <<'END'
command $(echo "foo") `echo "bar"`
END
__IN__
command $(echo "foo") `echo "bar"`
__OUT__

test_oE -e 0 'no arithmetic expansion with quoted here-document delimiter'
cat <<'END'
arithmetic $((1+10))
END
__IN__
arithmetic $((1+10))
__OUT__

test_oE -e 0 'no quote removal with quoted here-document delimiter'
cat <<'echo'
backslash \a \$foo \\\\ \`\` \"\" line-\
continuation
quote 'single' "double \$ 'a' " \$ 'a'
ec\
ho
echo
__IN__
backslash \a \$foo \\\\ \`\` \"\" line-\
continuation
quote 'single' "double \$ 'a' " \$ 'a'
ec\
ho
__OUT__

test_O -e n 'expansion error in here-document'
echo not printed <<END
${a?}
END
__IN__

test_oE -e 0 'various quotation of here-document delimiter'
cat <<E'N'D &&
single
END
cat <<E"N"D &&
double
END
cat <<E\ND
backslash
END
__IN__
single
double
backslash
__OUT__

test_oE -e 0 'tab removal with <<-'
cat <<-END
foo
			bar
		END
__IN__
foo
bar
__OUT__

test_oE -e 0 'here-document delimiter containing tab'
cat <<-END\	HERE
foo
	END	HERE
__IN__
foo
__OUT__

test_oE -e 0 'here-document delimiter starting with -'
cat << -END
foo
END
-END
__IN__
foo
END
__OUT__

test_oE -e 0 'multiple here-documents on single command'
foo=bar
{
    cat <&5
    cat <&4
    cat <&3
    cat
} <<END-0 3<<-END-3 4<<'END-4' 5<<-'END-5'
	0 $foo
END-0
	3 $foo
END-3
	4 $foo
END-4
	5 $foo
END-5
__IN__
5 $foo
	4 $foo
3 bar
	0 bar
__OUT__

test_oE -e 0 'multiple commands each with here-document'
cat <<END1; echo ---; cat <<END2
END2
END1
foo
END2
__IN__
END2
---
foo
__OUT__

test_o 'redirection is temporary' -e
{
    cat </dev/null
    cat
} <<END
here
END
__IN__
here
__OUT__

{
    echo 'exec cat <<END'
    i=0
    while [ $i -lt 10000 ]; do
       printf '%d\n' \
           $((i   )) $((i+ 1)) $((i+ 2)) $((i+ 3)) $((i+ 4)) $((i+ 5)) \
           $((i+ 6)) $((i+ 7)) $((i+ 8)) $((i+ 9)) $((i+10)) $((i+11)) \
           $((i+12)) $((i+13)) $((i+14)) $((i+15)) $((i+16)) $((i+17)) \
           $((i+18)) $((i+19)) $((i+20)) $((i+21)) $((i+22)) $((i+23)) \
           $((i+24)) $((i+25)) $((i+26)) $((i+27)) $((i+28)) $((i+29)) \
           $((i+30)) $((i+31)) $((i+32)) $((i+33)) $((i+34)) $((i+35)) \
           $((i+36)) $((i+37)) $((i+38)) $((i+39)) $((i+40)) $((i+41)) \
           $((i+42)) $((i+43)) $((i+44)) $((i+45)) $((i+46)) $((i+47)) \
           $((i+48)) $((i+49))
       i=$((i+50))
    done
    echo 'END'
} >longhere

test_oE -e 0 'long here-document' -e
. ./longhere |
while read -r i; do
    test "$i" -eq "${j:=0}"
    j=$((j+1))
done
__IN__
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
