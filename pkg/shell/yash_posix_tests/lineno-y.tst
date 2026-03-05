# lineno-y.tst: yash-specific test of the LINENO variable

test_oE -e 0 'LINENO and single quote'
echo $LINENO 'foo
bar' $LINENO
echo $LINENO 'foo
bar' $LINENO
__IN__
1 foo
bar 1
3 foo
bar 3
__OUT__

test_oE -e 0 'LINENO and double quote'
echo "$LINENO foo
bar $LINENO"
echo "$LINENO foo
bar $LINENO"
__IN__
1 foo
bar 1
3 foo
bar 3
__OUT__

test_oE -e 0 'LINENO and line continuation'
echo $LINENO \
    $LINENO \
    $LINENO
echo $LINENO \
    $LINENO \
    $LINENO
__IN__
1 1 1
4 4 4
__OUT__

test_oE -e 0 'LINENO and here-document (with expansion)'
cat <<END; echo c $LINENO
a $LINENO \
b $LINENO
END
cat <<END; echo f $LINENO
d $LINENO \
e $LINENO
END
__IN__
a 1 b 1
c 1
d 5 e 5
f 5
__OUT__

test_oE -e 0 'LINENO and here-document (without expansion)'
: <<\END
foo\
bar
baz
END
echo $LINENO
__IN__
6
__OUT__

test_oE -e 0 'LINENO in backquotes'
echo `echo $LINENO
echo $LINENO`
echo `echo $LINENO
echo $LINENO`
__IN__
1 2
1 2
__OUT__

test_oE -e 0 'LINENO in command substitution'
echo $(echo $LINENO
echo $LINENO)
echo $(echo $LINENO
echo $LINENO)
set -o posix # disallow pre-parsing of command substitution
echo $(echo $LINENO
echo $LINENO)
__IN__
1 2
3 4
1 2
__OUT__

test_oE -e 0 'LINENO in function'
echo a $LINENO
f() {
    echo b $LINENO
    echo c $LINENO
}
f
f
echo d $LINENO
__IN__
a 1
b 3
c 4
b 3
c 4
d 8
__OUT__

cat >dotscript <<\__END__
echo dot a $LINENO
echo dot b $LINENO

echo dot c $LINENO
__END__

test_oE -e 0 'LINENO in and out of dot script'
echo before $LINENO
. ./dotscript
echo after $LINENO
__IN__
before 1
dot a 1
dot b 2
dot c 4
after 3
__OUT__

test_oE -e 0 'LINENO in eval script'
echo before $LINENO
eval 'echo eval a $LINENO
echo eval b $LINENO

echo eval c $LINENO'

echo after $LINENO
__IN__
before 1
eval a 1
eval b 2
eval c 4
after 7
__OUT__

test_oE -e 0 'LINENO and alias with newline'
alias e='echo x $LINENO
echo y $LINENO'
echo a $LINENO
e
echo b $LINENO
__IN__
a 3
x 4
y 5
b 6
__OUT__

# In this test the character sequence "$((" looks like the beginning of an
# arithmetic expansion, but it does not have the corresponding "))", so the
# expansion is re-parsed as a command substitution.
test_oE -e 0 'LINENO after arithmetic-expansion-like command substitution' -s
: $(($(
\
)) )
echo $LINENO
__IN__
4
__OUT__
# ))

# XXX This is like the above, but it is much harder to fix...
: <<\__OUT__
test_oE -e 0 'LINENO in arithmetic-expansion-like command substitution' -s
echo $((echo $(
echo $LINENO \
)) )
echo $LINENO
__IN__
2
4
__OUT__
# ))

test_o -e 0 'LINENO in interactive shell is reset for each command line' -i +m
echo a $LINENO
for i in 1 2; do
    echo $i \
        $LINENO
done

echo b $LINENO

{
\
func () {
    echo f $LINENO
}
}
func
__IN__
a 1
1 2
2 2
b 1
f 4
__OUT__

test_oE -e 0 'exporting LINENO'
readonly LINENO # yash updates LINENO even if it is readonly
export LINENO
:
awk 'END { print ENVIRON["LINENO"] }' </dev/null
exec awk 'END { print ENVIRON["LINENO"] }' </dev/null
__IN__
4
5
__OUT__

test_oE -e 0 'assigning to LINENO'
LINENO=10
echo $LINENO
__IN__
10
__OUT__

test_oE -e 0 'unsetting LINENO'
unset LINENO
LINENO=10
echo $LINENO
__IN__
10
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
