# hash-y.tst: yash-specific test of the hash built-in

# Tests for the -d option are omitted because we cannot test it portably.

# Prevent the echo command from being hashed for consistent results.
setup 'echo() (command echo "$@")'

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
test_oE 'remembering command path (by hash built-in)'
mkdir a b c
PATH=$PWD/a:$PWD/b:$PWD/c:$PATH
make_command b/command1 c/command1
hash command1
echo --- $?
make_command a/command1
command1
__IN__
--- 0
Running b/command1
__OUT__

export TEST_NO="$LINENO"
test_oE 'remembering command path (by executing command)'
mkdir a b c
PATH=$PWD/a:$PWD/b:$PWD/c:$PATH
make_command b/command1 c/command1
command1
echo ---
make_command a/command1
command1
__IN__
Running b/command1
---
Running b/command1
__OUT__

export TEST_NO="$LINENO"
test_oE 're-remembering command path'
mkdir a b c
PATH=$PWD/a:$PWD/b:$PWD/c:$PATH
make_command b/command1 c/command1
hash command1
echo ---
rm b/command1
hash command1
echo --- $?
make_command a/command1
command1
__IN__
---
--- 0
Running c/command1
__OUT__

export TEST_NO="$LINENO"
test_oE 'removing specific remembered command path'
mkdir a b c
PATH=$PWD/a:$PWD/b:$PWD/c:$PATH
make_command c/command1 c/command2
hash command1 command2
echo --- $?
make_command b/command1 b/command2
hash -r command1
make_command a/command1 a/command2
command1
command2
__IN__
--- 0
Running a/command1
Running c/command2
__OUT__

export TEST_NO="$LINENO"
test_oE 'removing all remembered command paths'
mkdir a b c
PATH=$PWD/a:$PWD/b:$PWD/c:$PATH
make_command c/command1 c/command2
hash command1
command2
make_command b/command1 b/command2
hash -r
echo --- $?
make_command a/command1 a/command2
command1
command2
__IN__
Running c/command2
--- 0
Running a/command1
Running a/command2
__OUT__

export TEST_NO="$LINENO"
test_oE 'remembering multiple command paths'
mkdir a b
PATH=$PWD/a:$PWD/b:$PATH
make_command b/command1 b/command2
hash command1 command2
echo --- $?
make_command a/command1 a/command2
command1
command2
__IN__
--- 0
Running b/command1
Running b/command2
__OUT__

export TEST_NO="$LINENO"
test_oE 'removing multiple remembered command paths'
mkdir a b c
PATH=$PWD/a:$PWD/b:$PWD/c:$PATH
make_command c/command1 c/command2
hash command1 command2
make_command b/command1 b/command2
hash -r command1 command2
echo --- $?
make_command a/command1 a/command2
command1
command2
__IN__
--- 0
Running a/command1
Running a/command2
__OUT__

export TEST_NO="$LINENO"
testcase "$LINENO" 'printing remembered commands (with -a)' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
mkdir a b
ln -s b c
PATH=$PWD/a:$PWD/c:$PATH
make_command a/command1 b/command2 b/hash
hash -r
hash hash command1 command2
echo ---
hash -a | sort
__IN__
---
$PWD/$TEST_NO.path/a/command1
$PWD/$TEST_NO.path/c/command2
$PWD/$TEST_NO.path/c/hash
__OUT__

test_x -e 0 'exit status of "hash -a"'
hash sh
hash -a
__IN__

export TEST_NO="$LINENO"
testcase "$LINENO" 'printing remembered commands (without -a)' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
mkdir a b
ln -s b c
PATH=$PWD/a:$PWD/c:$PATH
make_command a/command1 b/command2 b/hash
hash -r
hash hash command1 command2
echo ---
hash | sort
__IN__
---
$PWD/$TEST_NO.path/a/command1
$PWD/$TEST_NO.path/c/command2
__OUT__

test_x -e 0 'exit status of "hash"'
hash sh
hash
__IN__

)

test_OE -e 0 'assignment to $PATH removes all remembered command paths'
hash sh mkdir chmod
PATH= hash
__IN__

test_Oe -e 2 'using -a with operands'
hash -a foo
__IN__
hash: no operand is expected
__ERR__

test_Oe -e 2 'invalid option'
hash --no-such-option
__IN__
hash: `--no-such-option' is not a valid option
__ERR__
#'
#`

test_Oe -e 1 'slash in command name'
hash foo/bar
__IN__
hash: `foo/bar': a command name must not contain `/'
__ERR__
#'
#`
#'
#`

test_Oe -e 1 'empty command'
hash ''
__IN__
hash: command `' was not found in $PATH
__ERR__
#'
#`

test_Oe -e 1 'command not found'
PATH=.
hash _no_such_command_
__IN__
hash: command `_no_such_command_' was not found in $PATH
__ERR__
#'
#`

test_O -d -e 1 'printing to closed stream'
hash sh
hash >&-
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
