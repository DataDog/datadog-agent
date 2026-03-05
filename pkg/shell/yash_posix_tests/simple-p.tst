# simple-p.tst: test of simple commands for any POSIX-compliant shell

posix="true"

setup -d

test_x -e 11 'exit status of empty command with command substitution'
$(exit 11)
__IN__

test_oE 'command words are expanded before assignments and redirections'
unset a
a=$(echo A 3>|f1) 3>|f1 echo "$(test -f f1 || echo file does not exist $a)"
__IN__
file does not exist
__OUT__

test_OE -e 0 'redirections precedes assignments for non-special-builtin command'
rm -f f2
a=$(cat f2) 3>|$(echo f2) true
__IN__

test_oE 'single assignment'
a=
bracket "$a"
a=value
bracket "$a"
__IN__
[]
[value]
__OUT__

test_oE 'multiple assignments'
a= b=bar
c=$b d=X e=$a
bracket "$c" "$d" "$e"
__IN__
[bar][X][]
__OUT__

test_oE 'assignments are subject to expansion'
x=X
a=$x${x} b=$(echo $x)`echo $x` c=$((1+2))
bracket "$a" "$b" "$c"
__IN__
[XX][XX][3]
__OUT__
# Tilde expansion is tested in tilde-p.tst

test_oE 'quotes in assignment'
a='A"B"C' b="A'B'C" c=\'\"C\"\'
bracket "$a" "$b" "$c"
__IN__
[A"B"C][A'B'C]['"C"']
__OUT__

test_oE 'assignment is persistent for empty command'
unset a b x
a=A
b=B $x
bracket "$a" "$b"
__IN__
[A][B]
__OUT__

# Tested in builtins-p.tst
#test_oE 'assignment is persistent for special built-in'

test_oE 'assigned variable is visible inside function'
f() { echo function $a; }
a=1
a=2 f
__IN__
function 2
__OUT__

test_oE 'assignment is temporary for regular command'
a=1
a=2 echo ok
bracket "$a"
__IN__
ok
[1]
__OUT__

test_oE 'assignment is exported for regular command'
a=A sh -c 'echo $a'
__IN__
A
__OUT__

test_O -d -e n 'assigning to read-only variable: exit with message (empty)'
readonly a=A
a=B
echo not reached
__IN__

test_O -d -e n 'assigning to read-only variable: exit with message (function)'
func() { echo not reached function; }
readonly a=A
a=B func
echo not reached command
__IN__

test_O -d -e n 'assigning to read-only variable in subshell'
readonly a=A
(a=B)
__IN__

test_x -e 0 'exit status of successful assignment'
a=1
__IN__

test_x -e 0 'exit status of successful redirection'
>/dev/null
__IN__

test_x -e 0 'exit status of successful assignments and redirections'
a=1 b=2 </dev/null >/dev/null
__IN__

test_x -e 13 'exit status of assignment with command substitution'
a=$(exit 13)
__IN__

test_o 'assignment is done even if command substitution fails (+e)' +e
a=foo$(false)
bracket "$a"
__IN__
[foo]
__OUT__

test_o 'assignment is done even if command substitution fails (-e)' -e
trap 'bracket "$a"' EXIT
a=foo$(false)
echo not reached
__IN__
[foo]
__OUT__

test_x -e 17 'exit status of redirection with command substitution'
>/dev/null$(exit 17)
__IN__

test_x -e 0 'redirection is done even if command substitution fails (+e)' +e
>f11$(false)
[ -f f11 ]
__IN__

test_o 'redirection is done even if command substitution fails (-e)' -e
trap '[ -f f12 ] && echo f12 created' EXIT
>f12$(false)
__IN__
f12 created
__OUT__

test_o 'assignment-like command argument'
export foo=F
sh -c 'echo $1 $foo' X foo=bar
foo=f sh -c 'echo $1 $foo' X foo=bar
__IN__
foo=bar F
foo=bar f
__OUT__

test_o 'redirection can appear between any tokens in simple command'
</dev/null foo=bar </dev/null sh </dev/null -c 'echo $1' X </dev/null 1 </dev/null
__IN__
1
__OUT__

test_O -d -e 127 'non-intrinsic command echo is not found w/o PATH'
PATH=
echo not printed
__IN__

mkdir dir1 dir2 dir3
cat >dir2/ext_cmd <<\END
echo external
echo command
printf '[%s]\n' "$@"
END
chmod a+x dir2/ext_cmd
ln -s "$(command -v sh)" dir2/link_to_sh

test_o 'searching PATH for command'
PATH=./dir1:./dir2:./dir3:$PATH
ext_cmd argument ' 1  2 '
__IN__
external
command
[argument]
[ 1  2 ]
__OUT__

test_O -d -e 127 'command not found in PATH'
PATH=./dir3
ext_cmd
__IN__

test_O -d -e 127 'PATH is searched after assignments (ls)'
# If PATH is searched before assignments,
# this would find ls in somewhere like /bin.
PATH=./dir3 ls
__IN__

test_O -d -e 127 'PATH is searched after assignments (pwd)'
# If PATH is searched before assignments,
# this would find pwd in somewhere like /bin.
PATH=./dir3 pwd
__IN__

test_o 'command name with slash'
dir2/ext_cmd foo bar baz
__IN__
external
command
[foo]
[bar]
[baz]
__OUT__

(
# Ensure $PWD is safe to assign to $PATH
case $PWD in (*[:%]*)
    skip="true"
esac

setup - <<\__END__
mkdir "$TEST_NO.path" && cd "$TEST_NO.path"
make_command() for c do echo echo "Running $c" >"$c" && chmod a+x "$c"; done
__END__

export TEST_NO="$LINENO"
test_oE 'running command in different directory with relative path in $PATH'
mkdir a b
make_command a/command1 b/command1
PATH=.:$PATH
cd a
command1
cd ../b
command1
__IN__
Running a/command1
Running b/command1
__OUT__

export TEST_NO="$LINENO"
test_oE 'assignment to $PATH removes all remembered command paths'
mkdir a b c
PATH=$PWD/a:$PWD/b:$PWD/c:$PATH
make_command c/command1 c/command2
command1
command2
echo ---
make_command b/command1 b/command2
PATH="$PATH"
make_command a/command1 a/command2
command1
command2
__IN__
Running c/command1
Running c/command2
---
Running a/command1
Running a/command2
__OUT__

export TEST_NO="$LINENO"
test_oE 'remembered command path is ignored if command is missing'
mkdir a b
PATH=$PWD/a:$PWD/b:$PATH
make_command b/command1 b/command2
command1
command2
echo ---
make_command a/command1 a/command2; rm b/command1 b/command2
command1
command2
__IN__
Running b/command1
Running b/command2
---
Running a/command1
Running a/command2
__OUT__

)

test_o 'argv[0] (command name without slash)'
sh -c 'echo "$0"'
PATH=./dir2:$PATH
link_to_sh -c 'echo "$0"'
__IN__
sh
link_to_sh
__OUT__

testcase "$LINENO" 'argv[0] (command name with slash)' \
    3<<\__IN__ 4<<__OUT__ 5<&-
"$(command -v sh)" -c 'echo "$0"'
./dir2/link_to_sh -c 'echo "$0"'
__IN__
$(command -v sh)
./dir2/link_to_sh
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
