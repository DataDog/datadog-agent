# history1-y.tst: yash-specific test of history, part 1

if ! testee -c 'command -bv fc history' >/dev/null; then
    skip="true"
fi

cat >rcfile1 <<\__END__
PS1= PS2= HISTFILE=$PWD/$histfile HISTSIZE=$histsize
unset HISTRMDUP
__END__

cat >rcfile2 <<\__END__
PS1= PS2= HISTFILE=$PWD/$histfile HISTSIZE=$histsize HISTRMDUP=$histrmdup
__END__

(
export histfile=histfile$LINENO histsize=50

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
__END__

test_OE -e 0 'history is saved when shell exits' -i +m --rcfile="rcfile1"
test -s $histfile
__IN__

)

(
export histfile=histfile$LINENO histsize=100

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1

echo foo 2
 	
echo foo 3
__END__

test_oE -e 0 'empty lines are not saved in history' -i +m --rcfile="rcfile1"
fc -l
__IN__
1	echo foo 1
2	echo foo 2
3	echo foo 3
4	fc -l
__OUT__

)

(
export histfile=histfile$LINENO histsize=50

test_OE -e 0 'listing empty history (-l)' -i +m --rcfile="rcfile1"
history -c; fc -l
__IN__

)

(
export histfile=histfile$LINENO histsize=50

test_Oe -e 1 're-execution with empty history (-s)' -i +m --rcfile="rcfile1"
history -c; fc -s
__IN__
fc: the command history is empty
__ERR__

)

(
export histfile=histfile$LINENO histsize=100

test_oE 'HISTRMDUP unset' -i +m --rcfile="rcfile1"
echo foo
echo foo
echo foo
fc -l
__IN__
foo
foo
foo
1	echo foo
2	echo foo
3	echo foo
4	fc -l
__OUT__

)

(
export histfile=histfile$LINENO histsize=100 histrmdup=x

test_oE 'HISTRMDUP not a number' -i +m --rcfile="rcfile2"
echo foo
echo foo
echo foo
fc -l
__IN__
foo
foo
foo
1	echo foo
2	echo foo
3	echo foo
4	fc -l
__OUT__

)

(
export histfile=histfile$LINENO histsize=100 histrmdup=0

test_oE 'HISTRMDUP=0' -i +m --rcfile="rcfile2"
echo foo
echo foo
echo foo
fc -l
__IN__
foo
foo
foo
1	echo foo
2	echo foo
3	echo foo
4	fc -l
__OUT__

)

(
export histfile=histfile$LINENO histsize=100 histrmdup=1

test_oE 'HISTRMDUP=1' -i +m --rcfile="rcfile2"
echo foo
echo foo
fc -l
echo foo
fc -l
__IN__
foo
foo
1	echo foo
2	fc -l
foo
1	echo foo
2	fc -l
3	echo foo
4	fc -l
__OUT__

)

(
export histfile=histfile$LINENO histsize=100 histrmdup=2

test_oE 'HISTRMDUP=2' -i +m --rcfile="rcfile2"
echo foo
echo foo
fc -l
echo foo
fc -l
echo bar
echo foo
fc -l
__IN__
foo
foo
1	echo foo
2	fc -l
foo
3	echo foo
4	fc -l
bar
foo
3	echo foo
4	fc -l
5	echo bar
6	echo foo
7	fc -l
__OUT__

)

(
export histfile=histfile$LINENO histsize=100 histrmdup=3

test_oE 'HISTRMDUP with similar commands' -i +m --rcfile="rcfile2"
echo foo
echo fo
echo f
fc -l
__IN__
foo
fo
f
1	echo foo
2	echo fo
3	echo f
4	fc -l
__OUT__

)

(
export histfile=histfile$LINENO histsize=5 histrmdup=5

test_oE 'HISTRMDUP=HISTSIZE' -i +m --rcfile="rcfile2"
fc -l
echo 2
echo 3
echo 4
echo 5
fc -l
__IN__
1	fc -l
2
3
4
5
2	echo 2
3	echo 3
4	echo 4
5	echo 5
6	fc -l
__OUT__

)

(
export histfile=histfile$LINENO histsize=5 histrmdup=6

test_oE 'HISTRMDUP=HISTSIZE+1' -i +m --rcfile="rcfile2"
fc -l
echo 2
echo 3
echo 4
echo 5
fc -l
__IN__
1	fc -l
2
3
4
5
2	echo 2
3	echo 3
4	echo 4
5	echo 5
6	fc -l
__OUT__

)

(
export histfile=/dev/null histsize=100

# History is remembered even if the history file is not a regular file.
test_oE 'null history file' -i +m --rcfile="rcfile1"
echo foo
history
__IN__
foo
1	echo foo
2	history
__OUT__

)

(
export histfile=inaccessible histsize=100
>"$histfile"
chmod a= "$histfile"

# History is remembered even if the history file is not a regular file.
test_oE 'null history file' -i +m --rcfile="rcfile1"
echo foo
history
__IN__
foo
1	echo foo
2	history
__OUT__

)

(
export histfile=/dev/null histsize=1

# If the /dev/stdin special file is available, use it to speed up the test.
# The shell enables buffering if it reads from a file.
if [ "$(echo ok | cat /dev/stdin 2>/dev/null)" = ok ]; then
    export STDIN=/dev/stdin
else
    unset STDIN
fi

test_OE -e 0 'history entry number wraps'
{
echo :
while echo fc -l; do
    :
done
} |
"$TESTEE" -i +m --rcfile="rcfile1" ${STDIN-} |
grep -Fqx '1	fc -l'
__IN__

)

(
export histfile=histfile$LINENO histsize=50

# Prepare the first history entry w/o running a test case.
[ "${skip-}" ] ||
    testee -is +m --rcfile="rcfile1" -o histspace >/dev/null <<\__END__
 echo foo 1
echo foo 2
 echo foo 3
echo foo 4
	echo foo 5
__END__

test_oE -e 0 'histspace option' -i +m --rcfile="rcfile1"
 : bar
fc -l
__IN__
1	echo foo 2
2	echo foo 4
3	 : bar
4	fc -l
__OUT__

)

# vim: set ft=sh ts=8 sts=4 sw=4 et:
