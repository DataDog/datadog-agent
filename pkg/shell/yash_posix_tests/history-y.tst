# history-y.tst: yash-specific test of the history built-in

if ! testee -c 'command -bv history' >/dev/null; then
    skip="true"
fi

test_oE -e 0 'history is an elective built-in'
command -V history
__IN__
history: an elective built-in
__OUT__

cat >rcfile1 <<\__END__
PS1= PS2= HISTFILE=$PWD/$histfile HISTSIZE=$histsize
unset HISTRMDUP
__END__

(
export histfile=histfile$LINENO histsize=100

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
echo foo 3
echo foo 4
echo foo 5
echo foo 6
echo foo 7
echo foo 8
echo foo 9
echo foo 10
echo foo 11
echo foo 12
echo foo 13
echo foo 14
echo foo 15
echo foo 16
echo foo 17
echo foo 18
echo foo 19
echo foo 20
__END__

test_oE -e 0 'whole history is printed by default' -i +m --rcfile="rcfile1"
history
__IN__
1	echo foo 1
2	echo foo 2
3	echo foo 3
4	echo foo 4
5	echo foo 5
6	echo foo 6
7	echo foo 7
8	echo foo 8
9	echo foo 9
10	echo foo 10
11	echo foo 11
12	echo foo 12
13	echo foo 13
14	echo foo 14
15	echo foo 15
16	echo foo 16
17	echo foo 17
18	echo foo 18
19	echo foo 19
20	echo foo 20
21	history
__OUT__

)

(
export histfile=histfile$LINENO histsize=100

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
echo foo 3
echo foo 4
echo foo 5
echo foo 6
echo foo 7
echo foo 8
echo foo 9
echo foo 10
__END__

test_oE -e 0 'printing specified number of entries' -i +m --rcfile="rcfile1"
history 7
__IN__
5	echo foo 5
6	echo foo 6
7	echo foo 7
8	echo foo 8
9	echo foo 9
10	echo foo 10
11	history 7
__OUT__

test_OE -e 0 'clearing history (-c)' -i +m --rcfile="rcfile1"
:
history -c; history
__IN__

test_OE -e 0 'clearing history (--clear)' -i +m --rcfile="rcfile1"
:
history --clear; history
__IN__

)

(
export histfile=histfile$LINENO histsize=100

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
echo foo 3
echo foo 4
__END__

test_oE -e 0 'deleting entry (-d)' -i +m --rcfile="rcfile1"
history -d 3; history
__IN__
1	echo foo 1
2	echo foo 2
4	echo foo 4
5	history -d 3; history
__OUT__

)

(
export histfile=histfile$LINENO histsize=100

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
echo foo 3
echo foo 4
echo foo 5
echo foo 6
__END__

test_oE -e 0 'deleting entries (-d, --delete)' -i +m --rcfile="rcfile1"
history -d 2 --delete=echo; history
__IN__
1	echo foo 1
3	echo foo 3
4	echo foo 4
5	echo foo 5
7	history -d 2 --delete=echo; history
__OUT__

test_Oe -e 1 'deleting non-existing entry (-d)' -i +m --rcfile="rcfile1"
history -d XXX
__IN__
history: no such history entry beginning with `XXX'
__ERR__
#`

# Essential part of the test of the -F option is missing because we cannot test
# it.

test_OE -e 0 'refreshing history file (-F)' -i +m --rcfile="rcfile1"
history -F
__IN__

test_OE -e 0 'refreshing history file (--flush-file)' -i +m --rcfile="rcfile1"
history --flush-file
__IN__

)

(
export in=./in$LINENO

cat >"$in" <<\__END__
echo bar 1\
echo bar 2
__END__

(
export histfile=histfile$LINENO histsize=100

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
__END__

test_oE -e 0 'reading entries (-r)' -i +m --rcfile="rcfile1"
history -r "$in"
history
__IN__
1	echo foo 1
2	echo foo 2
3	history -r "$in"
4	echo bar 1\
5	echo bar 2
6	history
__OUT__

)

(
export histfile=histfile$LINENO histsize=100

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
__END__

test_oE -e 0 'reading entries (--read)' -i +m --rcfile="rcfile1"
history --read="$in"
history
__IN__
1	echo foo 1
2	echo foo 2
3	history --read="$in"
4	echo bar 1\
5	echo bar 2
6	history
__OUT__

)

test_O -d -e 1 'reading commands from non-existing file'
history -r _no_such_file_
__IN__

)

(
export histfile=histfile$LINENO histsize=100

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
__END__

test_oE 'replacing entry (-s)' -i +m --rcfile="rcfile1"
history -s 'echo bar X'
echo [$?]
history
__IN__
[0]
1	echo foo 1
2	echo foo 2
3	echo bar X
4	echo [$?]
5	history
__OUT__

)

(
export histfile=histfile$LINENO histsize=100

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
__END__

test_oE 'replacing entry (--set)' -i +m --rcfile="rcfile1"
history --set='echo bar X'
echo [$?]
history
__IN__
[0]
1	echo foo 1
2	echo foo 2
3	echo bar X
4	echo [$?]
5	history
__OUT__

)

(
export histfile=histfile$LINENO histsize=100

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
__END__

test_oE 'replacing and adding entries (-s, --set)' -i +m --rcfile="rcfile1"
history -s 'echo bar X' --set='echo bar Y' -s 'echo bar Z'
echo [$?]
history
__IN__
[0]
1	echo foo 1
2	echo foo 2
3	echo bar X
4	echo bar Y
5	echo bar Z
6	echo [$?]
7	history
__OUT__

)

(
export histfile=histfile$LINENO histsize=100

test_oE 'adding entry to empty history (-s)' -i +m --rcfile="rcfile1"
history -c; history -s 'echo foo 1'
echo [$?]
history
__IN__
[0]
1	echo foo 1
2	echo [$?]
3	history
__OUT__

)

(
export histfile=histfile$LINENO histsize=100 out=./out$LINENO

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
__END__

test_oE 'writing entries to new file (-w)' -i +m --rcfile="rcfile1"
history -w "$out"
echo [$?]
cat "$out"
__IN__
[0]
echo foo 1
echo foo 2
history -w "$out"
__OUT__

)

(
export histfile=histfile$LINENO histsize=100 out=./out$LINENO

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
__END__

test_oE 'writing entries to new file (--write)' -i +m --rcfile="rcfile1"
history --write="$out"
echo [$?]
cat "$out"
__IN__
[0]
echo foo 1
echo foo 2
history --write="$out"
__OUT__

)

(
export histfile=histfile$LINENO histsize=100 out=./out$LINENO

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
__END__

cat >"$out" <<\__END__
This file is overwritten.
__END__

test_oE 'writing entries to existing file (-w)' -i +m --rcfile="rcfile1"
history -w "$out"
echo [$?]
cat "$out"
__IN__
[0]
echo foo 1
echo foo 2
history -w "$out"
__OUT__

)

(
export histfile=histfile$LINENO histsize=100 out=./out$LINENO

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
__END__

>"$out"
chmod a-w "$out"

# Skip if we're root.
if { echo >>"$out"; } 2>/dev/null; then
    skip="true"
fi

test_O -d -e 1 'writing entries to protected file (-w)' -i +m --rcfile="rcfile1"
history -w "$out"
__IN__

)

(
export histfile=histfile$LINENO histsize=100 in=./in$LINENO out=./out$LINENO

cat >"$in" <<\__END__
echo bar 1
echo bar 2
echo bar 3
__END__

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
echo foo 3
__END__

test_oE 'combination of options and operand' -i +m --rcfile="rcfile1"
history -d 2 -w "$out" -cF -r "$in" -s 'set' 2
echo [$?]
history
echo ---
cat "$out"
__IN__
2	echo bar 2
3	set
[0]
1	echo bar 1
2	echo bar 2
3	set
4	echo [$?]
5	history
---
echo foo 1
echo foo 3
history -d 2 -w "$out" -cF -r "$in" -s 'set' 2
__OUT__

)

test_Oe -e 2 'too many operands'
history 1 2
__IN__
history: too many operands are specified
__ERR__

test_Oe -e 2 'invalid option'
history --no-such-option
__IN__
history: `--no-such-option' is not a valid option
__ERR__
#`

(
export histfile=histfile$LINENO histsize=100

test_O -d -e 1 'printing to closed stream' -i +m --rcfile="rcfile1"
:
history >&-
__IN__

)

test_O -d -e 127 'history built-in is unavailable in POSIX mode' --posix
echo echo not reached > history
chmod a+x history
PATH=$PWD:$PATH
history --help
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
