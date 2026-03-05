# fc-y.tst: yash-specific test of the fc built-in

# Although the fc built-in is defined in POSIX, many aspects of its behavior
# are implementation-specific. That's why we don't have the fc-p.tst file.

if ! testee -c 'command -bv fc' >/dev/null; then
    skip="true"
fi

cat >rcfile1 <<\__END__
PS1= PS2= HISTFILE=$PWD/$histfile HISTSIZE=$histsize
unset HISTRMDUP
__END__

cat >rcfile2 <<\__END__
PS1= PS2= HISTFILE=$PWD/$histfile
unset HISTRMDUP HISTSIZE
__END__

test_oE -e 0 'fc is a mandatory built-in'
command -V fc
__IN__
fc: a mandatory built-in
__OUT__

(
export histfile=histfile$LINENO histsize=50

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
echo foo 3
__END__

test_oE -e 0 'listing commands in history (-l)' -i +m --rcfile="rcfile1"
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

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
echo foo 3
__END__

test_oE -e 0 'listing from specified command (-l)' -i +m --rcfile="rcfile1"
fc -l 2
__IN__
2	echo foo 2
3	echo foo 3
4	fc -l 2
__OUT__

)

(
export histfile=histfile$LINENO histsize=50

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
echo foo 3
echo foo 4
echo foo 5
__END__

test_oE -e 0 'listing range of commands, single (-l)' -i +m --rcfile="rcfile1"
fc -l 6
__IN__
6	fc -l 6
__OUT__

test_oE -e 0 'listing range of commands, many (-l)' -i +m --rcfile="rcfile1"
fc -l 2 4
__IN__
2	echo foo 2
3	echo foo 3
4	echo foo 4
__OUT__

test_oE -e 0 'listing descending range of commands (-l)' \
    -i +m --rcfile="rcfile1"
fc -l 2 1
__IN__
2	echo foo 2
1	echo foo 1
__OUT__

test_oE -e 0 'listing range of commands, reverse (-lr)' -i +m --rcfile="rcfile1"
fc -lr 2 4
__IN__
4	echo foo 4
3	echo foo 3
2	echo foo 2
__OUT__

test_oE -e 0 'listing descending range of commands, reverse (-lr)' \
    -i +m --rcfile="rcfile1"
fc -lr 4 2
__IN__
2	echo foo 2
3	echo foo 3
4	echo foo 4
__OUT__

)

(
export histfile=histfile$LINENO histsize=50

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
__END__

test_oE -e 0 'at most 16 commands are listed by default (-l)' \
    -i +m --rcfile="rcfile1"
fc -l
echo ---; fc -l
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
16	fc -l
---
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
16	fc -l
17	echo ---; fc -l
__OUT__

test_oE -e 0 'at most 16 commands are listed by default, reverse (-lr)' \
    -i +m --rcfile="rcfile1"
fc -lr
__IN__
18	fc -lr
17	echo ---; fc -l
16	fc -l
15	echo foo 15
14	echo foo 14
13	echo foo 13
12	echo foo 12
11	echo foo 11
10	echo foo 10
9	echo foo 9
8	echo foo 8
7	echo foo 7
6	echo foo 6
5	echo foo 5
4	echo foo 4
3	echo foo 3
__OUT__

test_oE -e 0 'listing without numbers (-lr)' -i +m --rcfile="rcfile1"
fc -ln 10 12
__IN__
	echo foo 10
	echo foo 11
	echo foo 12
__OUT__

)

# Test of the -v option is missing because we cannot control time.

(
export histfile=histfile$LINENO histsize=5

test_oE 'old entries are removed' -i +m --rcfile="rcfile1"
echo foo 1
echo foo 2
fc -l
echo foo 4
fc -l 1 6
fc -l 1 7
fc -l 1 8
__IN__
foo 1
foo 2
1	echo foo 1
2	echo foo 2
3	fc -l
foo 4
1	echo foo 1
2	echo foo 2
3	fc -l
4	echo foo 4
5	fc -l 1 6
2	echo foo 2
3	fc -l
4	echo foo 4
5	fc -l 1 6
6	fc -l 1 7
3	fc -l
4	echo foo 4
5	fc -l 1 6
6	fc -l 1 7
7	fc -l 1 8
__OUT__

export histsize=1

test_oE 'HISTSIZE=1' -i +m --rcfile="rcfile1"
fc -l
__IN__
2	fc -l
__OUT__

test_oE 'out-of-range history numbers are clamped, positive' \
    -i +m --rcfile="rcfile1"
fc -l 3 100
__IN__
2	fc -l 3 100
__OUT__

export histsize=0

test_oE 'HISTSIZE=0 is equal to HISTSIZE=1' -i +m --rcfile="rcfile1"
fc -l
__IN__
2	fc -l
__OUT__

)

(
export histfile=histfile$LINENO histsize=50

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
echo foo 3
echo foo 4
echo foo 5
__END__

test_oE 'listing with negative index (-l)' -i +m --rcfile="rcfile1"
fc -l -5 -3
__IN__
2	echo foo 2
3	echo foo 3
4	echo foo 4
__OUT__

test_oE 'out-of-range history numbers are clamped, negative' \
    -i +m --rcfile="rcfile1"
fc -l -100 -100
__IN__
1	echo foo 1
__OUT__

)

(
export histfile=histfile$LINENO histsize=50

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
echo foo 2
: echo foo 3
: X
__END__

test_oE 'identifying command by prefix' -i +m --rcfile="rcfile1"
fc -s ech
__IN__
echo foo 2
foo 2
__OUT__

test_Oe -e 1 'prefix matching no command' -i +m --rcfile="rcfile1"
fc -s XXX
__IN__
fc: no such history entry beginning with `XXX'
__ERR__
#`

)

(
export histfile=histfile$LINENO

test_oE 'default HISTSIZE is >= 128' -i +m --rcfile="rcfile1"
: foo 1
: foo 2
: foo 3
: foo 4
: foo 5
: foo 6
: foo 7
: foo 8
: foo 9
: foo 10
: foo 11
: foo 12
: foo 13
: foo 14
: foo 15
: foo 16
: foo 17
: foo 18
: foo 19
: foo 20
: foo 21
: foo 22
: foo 23
: foo 24
: foo 25
: foo 26
: foo 27
: foo 28
: foo 29
: foo 30
: foo 31
: foo 32
: foo 33
: foo 34
: foo 35
: foo 36
: foo 37
: foo 38
: foo 39
: foo 40
: foo 41
: foo 42
: foo 43
: foo 44
: foo 45
: foo 46
: foo 47
: foo 48
: foo 49
: foo 50
: foo 51
: foo 52
: foo 53
: foo 54
: foo 55
: foo 56
: foo 57
: foo 58
: foo 59
: foo 60
: foo 61
: foo 62
: foo 63
: foo 64
: foo 65
: foo 66
: foo 67
: foo 68
: foo 69
: foo 70
: foo 71
: foo 72
: foo 73
: foo 74
: foo 75
: foo 76
: foo 77
: foo 78
: foo 79
: foo 80
: foo 81
: foo 82
: foo 83
: foo 84
: foo 85
: foo 86
: foo 87
: foo 88
: foo 89
: foo 90
: foo 91
: foo 92
: foo 93
: foo 94
: foo 95
: foo 96
: foo 97
: foo 98
: foo 99
: foo 100
: foo 101
: foo 102
: foo 103
: foo 104
: foo 105
: foo 106
: foo 107
: foo 108
: foo 109
: foo 110
: foo 111
: foo 112
: foo 113
: foo 114
: foo 115
: foo 116
: foo 117
: foo 118
: foo 119
: foo 120
: foo 121
: foo 122
: foo 123
: foo 124
: foo 125
: foo 126
: foo 127
fc -l 1 3; fc -l 128
__IN__
1	: foo 1
2	: foo 2
3	: foo 3
128	fc -l 1 3; fc -l 128
__OUT__

)

(
export histfile=histfile$LINENO histsize=50

# This is yash-specific behavior; POSIX allows a history entry to have more
# than one line.
test_oE 'multi-line command' -i +m --rcfile="rcfile1"
echo \
foo
fc -l
__IN__
foo
1	echo \
2	foo
3	fc -l
__OUT__

)

(
export histfile=histfile$LINENO histsize=50

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
__END__

test_oE 're-executing command w/o editing (-s)' -i +m --rcfile="rcfile1"
fc -s 1
__IN__
echo foo 1
foo 1
__OUT__

)

(
export histfile=histfile$LINENO histsize=50

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
__END__

test_oE 'default operand for -s is -1' -i +m --rcfile="rcfile1"
fc -s
__IN__
echo foo 1
foo 1
__OUT__

)

(
export histfile=histfile$LINENO histsize=50

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo/bar/foo/bar
__END__

test_oE 'replacing part of re-executed command, default command (-s)' \
    -i +m --rcfile="rcfile1"
fc -s oo=xx
fc -s 1=2 # no match, no replacement
__IN__
echo fxx/bar/foo/bar
fxx/bar/foo/bar
echo fxx/bar/foo/bar
fxx/bar/foo/bar
__OUT__

test_oE 'replacing part of re-executed command, non-default command (-s)' \
    -i +m --rcfile="rcfile1"
fc -s oo=zz 1
__IN__
echo fzz/bar/foo/bar
fzz/bar/foo/bar
__OUT__

)

(
export histfile=histfile$LINENO histsize=50

if ! [ "${skip-}" ]; then
    # Prepare the first history entries w/o running a test case.
    testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
fc -s 1=2
__END__
fi

test_oE 're-executed command is saved in history (-s)' -i +m --rcfile="rcfile1"
fc -l
__IN__
1	echo foo 1
2	echo foo 2
3	fc -l
__OUT__

test_oE 'suppressing reprinting of re-executed command (-qs)' \
    -i +m --rcfile="rcfile1"
fc -qs 1
__IN__
foo 1
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
echo foo 11
echo foo 12
echo foo 13
echo foo 14
__END__

test_oE 're-executing single command after editing' -i +m --rcfile="rcfile1"
FCEDIT=true fc 2
__IN__
echo foo 2
foo 2
__OUT__

test_oE 're-executing range of commands after editing' -i +m --rcfile="rcfile1"
FCEDIT=true fc 2 4
__IN__
echo foo 2
echo foo 3
echo foo 4
foo 2
foo 3
foo 4
__OUT__

test_oE 're-executing descending range of commands after editing' \
    -i +m --rcfile="rcfile1"
FCEDIT=true fc 4 2
__IN__
echo foo 4
echo foo 3
echo foo 2
foo 4
foo 3
foo 2
__OUT__

test_oE 're-executing reverse range of commands after editing' \
    -i +m --rcfile="rcfile1"
FCEDIT=true fc -r 2 4
__IN__
echo foo 4
echo foo 3
echo foo 2
foo 4
foo 3
foo 2
__OUT__

test_oE 're-executing reverse descending range of commands after editing' \
    -i +m --rcfile="rcfile1"
FCEDIT=true fc -r 4 2
__IN__
echo foo 2
echo foo 3
echo foo 4
foo 2
foo 3
foo 4
__OUT__

test_oE 'last command is edited by default (w/o -r)' -i +m --rcfile="rcfile1"
echo foo X
FCEDIT=true fc
__IN__
foo X
echo foo X
foo X
__OUT__

test_oE 'last command is edited by default (with -r)' -i +m --rcfile="rcfile1"
echo foo X
FCEDIT=true fc -r
__IN__
foo X
echo foo X
foo X
__OUT__

test_oE 'suppressing reprinting of re-executed command (-q)' \
    -i +m --rcfile="rcfile1"
FCEDIT=true fc -q 1
__IN__
foo 1
__OUT__

# The first 10 lines of the edited commands are first printed by "head".
# Then "fc" prints the commands before executing.
test_oE '$FCEDIT specifies editor' -i +m --rcfile="rcfile1"
FCEDIT=head fc 2 13
__IN__
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
foo 2
foo 3
foo 4
foo 5
foo 6
foo 7
foo 8
foo 9
foo 10
foo 11
foo 12
foo 13
__OUT__

test_oE 'option overrides $FCEDIT' -i +m --rcfile="rcfile1"
FCEDIT=true fc -e head 2 13
__IN__
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
foo 2
foo 3
foo 4
foo 5
foo 6
foo 7
foo 8
foo 9
foo 10
foo 11
foo 12
foo 13
__OUT__

)

(
export histfile=histfile$LINENO histsize=100

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null 2>&1 <<\__END__
echo stdout
echo stderr >&2
cat
__END__

test_oe 'redirection affects editor and re-executed command' \
    -i +m --rcfile="rcfile1"
fc -e cat 1 3 3>&1 1>&2 2>&3 <<\_IN_
stdin
_IN_
__IN__
stderr
__OUT__
echo stdout
echo stderr >&2
cat
echo stdout
echo stderr >&2
cat
stdout
stdin
__ERR__

)

(
export histfile=histfile$LINENO histsize=100 editor=./editor$LINENO

cat >"$editor" <<\__END__
echo $x
__END__
chmod a+x "$editor"

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null 2>&1 <<\__END__
"$editor"
__END__

test_oE 'assignment affects editor and re-executed command' \
    -i +m --rcfile="rcfile1"
x=test_assignment fc -e "$editor"
__IN__
test_assignment
"$editor"
test_assignment
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
__END__

# Some ed implementations seem broken in that their "w" command prints the byte
# count to stderr rather than stdout. The redirection works around this in the
# test case below.
test_oE 'default editor is ed' -i +m --rcfile="rcfile1"
fc 2 4 2>&1 <<\__ED__
2d
1a
echo bar X
.
w
__ED__
__IN__
33
33
echo foo 2
echo bar X
echo foo 4
foo 2
bar X
foo 4
__OUT__

)

(
export histfile=histfile$LINENO histsize=100 editor=./editor$LINENO

cat >"$editor" <<\__END__
rm -- "$1" && echo echo bar 2 >"$1"
__END__

# Prepare the first history entry w/o running a test case.
testee -is +m --rcfile="rcfile1" >/dev/null <<\__END__
echo foo 1
__END__
chmod a+x "$editor"

# In this test, the editor removes the file and creates a new file with the
# same name but with a (normally) different i-node. The shell must read the new
# file successfully.
test_oE 're-creation of command file' -i +m --rcfile="rcfile1"
fc -e "$editor"
__IN__
echo bar 2
bar 2
__OUT__

)

(
export histfile=histfile$LINENO histsize=100 editor=./editor$LINENO

test_oe -e n 'editor returning non-zero prevents command execution' \
    -i +m --rcfile="rcfile1"
echo foo
fc -e false
__IN__
foo
__OUT__
fc: the editor returned a non-zero exit status
__ERR__

)

(
export histfile=histfile$LINENO histsize=100 editor=./editor$LINENO

test_oE '$? in executed command' -i +m --rcfile="rcfile1"
true
echo $?
(exit 37)
fc -e true 2
__IN__
0
echo $?
37
__OUT__

)

(
export histfile=histfile$LINENO histsize=100 editor=./editor$LINENO

test_x -e 17 'exit status of command re-execution' -i +m --rcfile="rcfile1"
(exit 17)
fc -e true
__IN__

)

(
export histfile=histfile$LINENO histsize=100 editor=./editor$LINENO

test_oE 'interaction between shells' -i +m --rcfile="rcfile1"
: foo 1
echo : bar 3 | "$TESTEE" -i +m --rcfile="rcfile1"
: foo 4
fc -l
__IN__
1	: foo 1
2	echo : bar 3 | "$TESTEE" -i +m --rcfile="rcfile1"
3	: bar 3
4	: foo 4
5	fc -l
__OUT__

)

test_Oe -e 2 'invalid option (non-existing option)'
fc --no-such-option
__IN__
fc: `--no-such-option' is not a valid option
__ERR__
#`

test_Oe -e 2 'invalid option (-e and -l)'
fc -e xxx -l
__IN__
fc: the -e option cannot be used with the -l option
__ERR__

test_Oe -e 2 'invalid option (-e and -s)'
fc -e xxx -s
__IN__
fc: the -e option cannot be used with the -s option
__ERR__

test_Oe -e 2 'invalid option (-l and -q)'
fc -lq
__IN__
fc: the -l option cannot be used with the -q option
__ERR__

test_Oe -e 2 'invalid option (-l and -s)'
fc -ls
__IN__
fc: the -l option cannot be used with the -s option
__ERR__

test_Oe -e 2 'invalid option (-r and -s)'
fc -rs
__IN__
fc: the -r option cannot be used with the -s option
__ERR__

test_Oe -e 2 'invalid option (-n w/o -l)'
fc -n
__IN__
fc: the -n or -v option must be used with the -l option
__ERR__

test_Oe -e 2 'invalid option (-v w/o -l)'
fc -v
__IN__
fc: the -n or -v option must be used with the -l option
__ERR__

test_Oe -e 2 'too many operands (w/o -l or -s)'
fc 1 2 3
__IN__
fc: too many operands are specified
__ERR__

test_Oe -e 2 'too many operands (-l)'
fc -l 1 2 3
__IN__
fc: too many operands are specified
__ERR__

test_Oe -e 2 'too many operands (-s, w/ substitution)'
fc -s 1 2
__IN__
fc: too many operands are specified
__ERR__

test_Oe -e 2 'too many operands (-s, w/o substitution)'
fc -s old=new 1 2
__IN__
fc: too many operands are specified
__ERR__

(
export histfile=histfile$LINENO histsize=100

test_O -d -e 1 'printing to closed stream' -i +m --rcfile="rcfile1"
:
fc -l >&-
__IN__

)

# vim: set ft=sh ts=8 sts=4 sw=4 et:
