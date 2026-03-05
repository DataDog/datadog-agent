# dirstack-y.tst: yash-specific test of directory stack

if ! testee -c 'command -bv pushd' >/dev/null; then
    skip="true"
fi

cd -P . # make $PWD a physical path

mkdir testdir testdir/1 testdir/2 testdir/3
mkdir -m 000 testdir/000
ln -s .. testdir/parent

##### dirs

test_oE -e 0 'dirs is an elective built-in'
command -V dirs
__IN__
dirs: an elective built-in
__OUT__

testcase "$LINENO" -e 0 'printing unset directory stack' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
unset DIRSTACK
dirs
__IN__
$PWD
__OUT__

testcase "$LINENO" -e 0 'printing empty directory stack' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=()
dirs
__IN__
$PWD
__OUT__

testcase "$LINENO" -e 0 'printing non-array directory stack' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK="$PWD:$PWD"
dirs
__IN__
$PWD
__OUT__

testcase "$LINENO" -e 0 'printing directory stack (2 directories)' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=(/foo)
dirs
__IN__
$PWD
/foo
__OUT__

testcase "$LINENO" -e 0 'printing directory stack (4 directories)' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=(/foo "/name with space" /bar/baz)
dirs
__IN__
$PWD
/bar/baz
/name with space
/foo
__OUT__

testcase "$LINENO" -e 0 'printing part of directory stack' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=(/4 /3 /2 /1)
dirs -0 +0 -2
__IN__
/4
$PWD
/2
__OUT__

test_oE -e 0 'printing directory stack with false $PWD'
unset DIRSTACK
PWD="/_no_such_directory_"
dirs
__IN__
/_no_such_directory_
__OUT__

testcase "$LINENO" -e 0 'printing directory stack (-v)' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=(/foo "/name with space" /bar/baz)
dirs -v
__IN__
+0	-3	$PWD
+1	-2	/bar/baz
+2	-1	/name with space
+3	-0	/foo
__OUT__

testcase "$LINENO" -e 0 'printing directory stack (--verbose)' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=(/foo "/name with space" /bar/baz)
dirs --verbose
__IN__
+0	-3	$PWD
+1	-2	/bar/baz
+2	-1	/name with space
+3	-0	/foo
__OUT__

testcase "$LINENO" -e 0 'printing part of directory stack (-v)' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=(/4 /3 /2 /1)
dirs -v -0 +0 -2
__IN__
+4	-0	/4
+0	-4	$PWD
+2	-2	/2
__OUT__

test_Oe -e n 'dirs: index out of range (+1 for 1 directory)'
unset DIRSTACK
dirs +1
__IN__
dirs: the directory stack is empty
__ERR__

test_Oe -e n 'dirs: index out of range (-1 for 1 directory)'
unset DIRSTACK
dirs -1
__IN__
dirs: the directory stack is empty
__ERR__

test_Oe -e n 'dirs: index out of range (+2 for 2 directories)'
DIRSTACK=(/foo)
dirs +2
__IN__
dirs: index +2 is out of range
__ERR__

test_Oe -e n 'dirs: index out of range (-2 for 2 directories)'
DIRSTACK=(/foo)
dirs -2
__IN__
dirs: index -2 is out of range
__ERR__

test_Oe -e n 'dirs: invalid index string'
dirs 0
__IN__
dirs: `0' is not a valid index
__ERR__
#`

test_O -d -e n 'dirs: printing to closed stream'
dirs >&-
__IN__

test_oE -e 0 'clearing directory stack (-c)'
DIRSTACK=(a b c)
dirs -c && echo "${DIRSTACK-unset}"
__IN__
unset
__OUT__

test_oE -e 0 'clearing directory stack (--clear)'
DIRSTACK=(a b c)
dirs --clear && echo "${DIRSTACK-unset}"
__IN__
unset
__OUT__

test_Oe -e n 'clearing read-only directory stack'
DIRSTACK=(a)
readonly DIRSTACK
dirs -c
__IN__
dirs: $DIRSTACK is read-only
__ERR__

test_Oe -e n 'dirs: invalid option'
dirs --no-such-option
__IN__
dirs: `--no-such-option' is not a valid option
__ERR__
#`

test_O -d -e 127 'dirs built-in is unavailable in POSIX mode' --posix
echo echo not reached > dirs
chmod a+x dirs
PATH=$PWD:$PATH
dirs --help
__IN__

##### pushd

test_oE -e 0 'pushd is an elective built-in'
command -V pushd
__IN__
pushd: an elective built-in
__OUT__

testcase "$LINENO" 'pushing directory' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=(a b c)
pushd testdir
echo $?
pwd
dirs
__IN__
0
$PWD/testdir
$PWD/testdir
$PWD
c
b
a
__OUT__

testcase "$LINENO" 'pushing to empty stack' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
pushd testdir
echo $?
pwd
dirs
__IN__
0
$PWD/testdir
$PWD/testdir
$PWD
__OUT__

testcase "$LINENO" 'pushing with false $PWD' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=(a b c) PWD=X
pushd testdir
echo $?
pwd
dirs
__IN__
0
$PWD/testdir
$PWD/testdir
X
c
b
a
__OUT__

testcase "$LINENO" 'pushing logical path, default' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=("$PWD/testdir/1" "$PWD/testdir/parent/testdir/2")
pushd testdir/parent/testdir/3
echo $?
pwd
dirs
__IN__
0
$PWD/testdir/parent/testdir/3
$PWD/testdir/parent/testdir/3
$PWD
$PWD/testdir/parent/testdir/2
$PWD/testdir/1
__OUT__

testcase "$LINENO" 'pushing logical path, short option' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=("$PWD/testdir/1" "$PWD/testdir/parent/testdir/2")
pushd -L testdir/parent/testdir/3
echo $?
pwd
dirs
__IN__
0
$PWD/testdir/parent/testdir/3
$PWD/testdir/parent/testdir/3
$PWD
$PWD/testdir/parent/testdir/2
$PWD/testdir/1
__OUT__

testcase "$LINENO" 'pushing logical path, long option' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=("$PWD/testdir/1" "$PWD/testdir/parent/testdir/2")
pushd --logical testdir/parent/testdir/3
echo $?
pwd
dirs
__IN__
0
$PWD/testdir/parent/testdir/3
$PWD/testdir/parent/testdir/3
$PWD
$PWD/testdir/parent/testdir/2
$PWD/testdir/1
__OUT__

testcase "$LINENO" 'pushing logical path, both options' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=("$PWD/testdir/1" "$PWD/testdir/parent/testdir/2")
pushd -PL testdir/parent/testdir/3
echo $?
pwd
dirs
__IN__
0
$PWD/testdir/parent/testdir/3
$PWD/testdir/parent/testdir/3
$PWD
$PWD/testdir/parent/testdir/2
$PWD/testdir/1
__OUT__

testcase "$LINENO" 'pushing physical path, short option' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=("$PWD/testdir/1" "$PWD/testdir/parent/testdir/2")
pushd -P testdir/parent/testdir/3
echo $?
pwd
dirs
__IN__
0
$PWD/testdir/3
$PWD/testdir/3
$PWD
$PWD/testdir/parent/testdir/2
$PWD/testdir/1
__OUT__

testcase "$LINENO" 'pushing physical path, long option' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=("$PWD/testdir/1" "$PWD/testdir/parent/testdir/2")
pushd --physical testdir/parent/testdir/3
echo $?
pwd
dirs
__IN__
0
$PWD/testdir/3
$PWD/testdir/3
$PWD
$PWD/testdir/parent/testdir/2
$PWD/testdir/1
__OUT__

testcase "$LINENO" 'pushing physical path, both options' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=("$PWD/testdir/1" "$PWD/testdir/parent/testdir/2")
pushd -LP testdir/parent/testdir/3
echo $?
pwd
dirs
__IN__
0
$PWD/testdir/3
$PWD/testdir/3
$PWD
$PWD/testdir/parent/testdir/2
$PWD/testdir/1
__OUT__

testcase "$LINENO" 'pushing default path' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
pushd --default-directory="$PWD/testdir"
echo $?
pwd
dirs
__IN__
0
$PWD/testdir
$PWD/testdir
$PWD
__OUT__

testcase "$LINENO" 'pushing with ignored default path' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
pushd --default-directory=X testdir/1
echo $?
pwd
dirs
__IN__
0
$PWD/testdir/1
$PWD/testdir/1
$PWD
__OUT__

testcase "$LINENO" 're-pushing by default path' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=("$PWD/testdir" a)
pushd --default-directory=+2
echo $?
pwd
dirs
__IN__
0
$PWD/testdir
$PWD/testdir
$PWD
a
__OUT__

testcase "$LINENO" 'removing duplicates, normal case' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=(a "$PWD/testdir" b "$PWD/testdir" c)
pushd --remove-duplicates testdir
echo $?
pwd
dirs
__IN__
0
$PWD/testdir
$PWD/testdir
$PWD
c
b
a
__OUT__

testcase "$LINENO" 'removing duplicates, same directory' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
pushd --remove-duplicates .
echo $?
pwd
dirs
__IN__
0
$PWD
$PWD
__OUT__

testcase "$LINENO" 'removing duplicates, index' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=(a "$PWD/testdir" b "$PWD/testdir" c "$PWD/testdir")
pushd --remove-duplicates +1
echo $?
pwd
dirs
__IN__
0
$PWD/testdir
$PWD/testdir
$PWD
c
b
a
__OUT__

testcase "$LINENO" 'pushing $OLDPWD' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
OLDPWD="testdir"
pushd -
echo $?
pwd
dirs
__IN__
$PWD/testdir
0
$PWD/testdir
$PWD/testdir
$PWD
__OUT__

test_Oe -e 4 'missing $OLDPWD'
unset OLDPWD
pushd -
__IN__
pushd: $OLDPWD is not set
__ERR__

testcase "$LINENO" 'pushing positive index' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=(a b "$PWD/testdir" c)
pushd +2
echo $?
pwd
dirs
__IN__
0
$PWD/testdir
$PWD/testdir
$PWD
c
b
a
__OUT__

testcase "$LINENO" 'pushing negative index' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=(a "$PWD/testdir" b c)
pushd -1
echo $?
pwd
dirs
__IN__
0
$PWD/testdir
$PWD/testdir
$PWD
c
b
a
__OUT__

test_oE 'pushd: exported DIRSTACK' -a
pushd .
sh -c 'echo ${DIRSTACK:+set}'
__IN__
set
__OUT__

test_Oe -e 4 'pushd: index out of range (+1 for 1 directory)'
pushd +1
__IN__
pushd: the directory stack is empty
__ERR__

test_Oe -e 4 'pushd: index out of range (-1 for 1 directory)'
pushd -1
__IN__
pushd: the directory stack is empty
__ERR__

test_Oe -e 4 'pushd: index out of range (+2 for 2 directories)'
DIRSTACK=(/foo)
pushd +2
__IN__
pushd: index +2 is out of range
__ERR__

test_Oe -e 4 'pushd: index out of range (-2 for 2 directories)'
DIRSTACK=(/foo)
pushd -2
__IN__
pushd: index -2 is out of range
__ERR__

test_Oe -e 4 'pushd: default operand, empty stack'
pushd
__IN__
pushd: the directory stack is empty
__ERR__

testcase "$LINENO" 'pushd: default operand' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=("$PWD/testdir")
pushd
echo $?
pwd
dirs
__IN__
0
$PWD/testdir
$PWD/testdir
$PWD
__OUT__

# The exit status is success as long as the working directory and the stack are
# changed successfully.

testcase "$LINENO" 'pushd: read-only $PWD' \
    3<<\__IN__ 4<<__OUT__ 5<<\__ERR__
readonly PWD
pushd testdir
echo $?
pwd
dirs
__IN__
1
$PWD/testdir
$PWD
$PWD
__OUT__
pushd: $PWD is read-only
__ERR__

testcase "$LINENO" 'pushd: read-only $OLDPWD' \
    3<<\__IN__ 4<<__OUT__ 5<<\__ERR__
readonly OLDPWD=X
pushd testdir
echo $?
pwd
printf '[%s]\n' "$OLDPWD"
dirs
__IN__
1
$PWD/testdir
[X]
$PWD/testdir
$PWD
__OUT__
pushd: $OLDPWD is read-only
__ERR__

test_Oe -e 1 'pushd: read-only array $DIRSTACK, exit status and message'
DIRSTACK=(a)
readonly DIRSTACK
pushd testdir
__IN__
pushd: $DIRSTACK is read-only
__ERR__

testcase "$LINENO" 'pushd: read-only array $DIRSTACK, result stack' \
    3<<\__IN__ 4<<__OUT__ 5<&-
DIRSTACK=(a)
readonly DIRSTACK
pushd testdir
pwd
dirs
__IN__
$PWD/testdir
$PWD/testdir
a
__OUT__

test_Oe -e 1 'pushd: read-only non-array $DIRSTACK, exit status and message'
readonly DIRSTACK=
pushd testdir
__IN__
pushd: $DIRSTACK is not an array
__ERR__

testcase "$LINENO" 'pushd: read-only non-array $DIRSTACK, result stack' \
    3<<\__IN__ 4<<__OUT__ 5<&-
readonly DIRSTACK=
pushd testdir
pwd
dirs
__IN__
$PWD/testdir
$PWD/testdir
__OUT__

test_Oe -e 1 'pushd: read-only unset $DIRSTACK, exit status and message'
unset DIRSTACK
readonly DIRSTACK
pushd testdir
__IN__
pushd: $DIRSTACK is not an array
__ERR__

testcase "$LINENO" 'pushd: read-only unset $DIRSTACK, result stack' \
    3<<\__IN__ 4<<__OUT__ 5<&-
unset DIRSTACK
readonly DIRSTACK
pushd testdir
pwd
dirs
__IN__
$PWD/testdir
$PWD/testdir
__OUT__

test_Oe -e 4 'pushd: unset $PWD, exit status and message'
unset PWD
pushd testdir
__IN__
pushd: $PWD is not set
__ERR__

test_x "$LINENO" -e 0 'pushd: unset $PWD, directory not changed'
cd -P .
unset PWD
pushd testdir
test "$(pwd -P)" = "$OLDPWD"
__IN__

test_O -d -e 2 'pushing non-existing directory, exit status and message'
pushd _no_such_directory_
__IN__

testcase "$LINENO" 'pushing non-existing directory, result stack' \
    3<<\__IN__ 4<<__OUT__ 5<&-
pushd _no_such_directory_
pwd
dirs
__IN__
$PWD
$PWD
__OUT__

(
# A root user may have a special permission.
if [ -x testdir/000 ]; then
    skip="true"
fi

test_O -d -e 2 'pushing restricted directory'
pushd testdir/000
__IN__

)

test_OE -e 0 'pushd: printing to closed stream'
OLDPWD=$PWD
pushd - >&-
__IN__

test_Oe -e 5 'pushd: invalid option'
pushd --no-such-option
__IN__
pushd: `--no-such-option' is not a valid option
__ERR__
#`

test_Oe -e 5 -- '-e without -P'
pushd -e
__IN__
pushd: the -e option requires the -P option
__ERR__

test_Oe -e 5 'pushd: too many operands'
pushd +0 +0
__IN__
pushd: too many operands are specified
__ERR__

test_O -d -e 127 'pushd built-in is unavailable in POSIX mode' --posix
echo echo not reached > pushd
chmod a+x pushd
PATH=$PWD:$PATH
pushd --help
__IN__

##### popd

test_oE -e 0 'popd is an elective built-in'
command -V popd
__IN__
popd: an elective built-in
__OUT__

test_Oe -e 4 'popping default directory from empty stack'
popd
__IN__
popd: the directory stack is empty
__ERR__

testcase "$LINENO" 'popping default directory from 1-element stack' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=("$PWD/testdir")
popd
echo $?
pwd
dirs
__IN__
$PWD/testdir
0
$PWD/testdir
$PWD/testdir
__OUT__

testcase "$LINENO" 'popping default directory from 3-element stack' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=(a b "$PWD/testdir")
popd
echo $?
pwd
dirs
__IN__
$PWD/testdir
0
$PWD/testdir
$PWD/testdir
b
a
__OUT__

testcase "$LINENO" 'popping +0 from 1-element stack' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=("$PWD/testdir")
popd +0
echo $?
pwd
dirs
__IN__
$PWD/testdir
0
$PWD/testdir
$PWD/testdir
__OUT__

testcase "$LINENO" 'popping +0 from 3-element stack' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=(a b "$PWD/testdir")
popd +0
echo $?
pwd
dirs
__IN__
$PWD/testdir
0
$PWD/testdir
$PWD/testdir
b
a
__OUT__

test_Oe -e 4 'popping +0 from empty stack'
popd +0
__IN__
popd: the directory stack is empty
__ERR__

test_Oe -e 4 'popping +2 from empty stack'
popd +2
__IN__
popd: the directory stack is empty
__ERR__

test_Oe -e 4 'popping +2 from 1-element stack'
DIRSTACK=(b)
popd +2
__IN__
popd: index +2 is out of range
__ERR__

testcase "$LINENO" 'popping +2 from 3-element stack' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=(a b c)
popd +2
echo $?
pwd
dirs
__IN__
0
$PWD
$PWD
c
a
__OUT__

testcase "$LINENO" 'popping -1 from 3-element stack' \
    3<<\__IN__ 4<<__OUT__ 5</dev/null
DIRSTACK=(a b c)
popd -1
echo $?
pwd
dirs
__IN__
0
$PWD
$PWD
c
a
__OUT__

test_oE 'popd: exported DIRSTACK, +0'
pushd .
export DIRSTACK
popd +0 >/dev/null
sh -c 'echo ${DIRSTACK-unset}'
__IN__

__OUT__

test_oE 'popd: exported DIRSTACK, -0'
pushd .
export DIRSTACK
popd -0 >/dev/null
sh -c 'echo ${DIRSTACK-unset}'
__IN__

__OUT__

# The exit status is success as long as the working directory and the stack are
# changed successfully.

testcase "$LINENO" 'popd: read-only $PWD' \
    3<<\__IN__ 4<<__OUT__ 5<<\__ERR__
DIRSTACK=("$PWD/testdir")
readonly PWD
popd
__IN__
$PWD/testdir
__OUT__
popd: $PWD is read-only
__ERR__

testcase "$LINENO" 'popd: read-only $OLDPWD' \
    3<<\__IN__ 4<<__OUT__ 5<<\__ERR__
DIRSTACK=("$PWD/testdir")
readonly OLDPWD
popd
__IN__
$PWD/testdir
__OUT__
popd: $OLDPWD is read-only
__ERR__

test_Oe -e 4 'popd: read-only array $DIRSTACK, exit status and message'
DIRSTACK=("$PWD/testdir")
readonly DIRSTACK
popd
__IN__
popd: $DIRSTACK is read-only
__ERR__

testcase "$LINENO" 'popd: read-only array $DIRSTACK, result stack' \
    3<<\__IN__ 4<<__OUT__ 5<&-
DIRSTACK=("$PWD/testdir")
readonly DIRSTACK
popd
pwd
dirs
__IN__
$PWD
$PWD
$PWD/testdir
__OUT__

test_Oe -e 4 'popd: read-only non-array $DIRSTACK, exit status and message'
readonly DIRSTACK=
popd
__IN__
popd: $DIRSTACK is not an array
__ERR__

testcase "$LINENO" 'popd: read-only non-array $DIRSTACK, result stack' \
    3<<\__IN__ 4<<__OUT__ 5<&-
readonly DIRSTACK=
popd
pwd
dirs
__IN__
$PWD
$PWD
__OUT__

test_Oe -e 4 'popd: read-only unset $DIRSTACK, exit status and message'
unset DIRSTACK
readonly DIRSTACK
popd
__IN__
popd: $DIRSTACK is not an array
__ERR__

testcase "$LINENO" 'popd: read-only unset $DIRSTACK, result stack' \
    3<<\__IN__ 4<<__OUT__ 5<&-
unset DIRSTACK
readonly DIRSTACK
popd
pwd
dirs
__IN__
$PWD
$PWD
__OUT__

test_O -d -e 2 'popping to non-existing directory, exit status and message'
DIRSTACK=("$PWD/_no_such_directory_")
popd
__IN__

testcase "$LINENO" 'popping to non-existing directory, result stack' \
    3<<\__IN__ 4<<__OUT__ 5<&-
DIRSTACK=("$PWD/_no_such_directory_")
popd
pwd
dirs
__IN__
$PWD
$PWD
__OUT__

(
# A root user may have a special permission.
if [ -x testdir/000 ]; then
    skip="true"
fi

test_O -d -e 2 'popping to restricted directory'
DIRSTACK=("$PWD/testdir/000")
popd
__IN__

)

test_OE -e 0 'popd: printing to closed stream'
DIRSTACK=("$PWD")
popd >&-
__IN__

test_Oe -e 5 'popd: invalid option'
popd --no-such-option
__IN__
popd: `--no-such-option' is not a valid option
__ERR__
#`

test_Oe -e 5 'popd: too many operands'
popd +0 +0
__IN__
popd: too many operands are specified
__ERR__

test_Oe -e 5 'popd: non-numeric operand'
DIRSTACK=("$PWD")
popd not-a-number
__IN__
popd: `not-a-number' is not a valid index
__ERR__
#'
#`

test_O -d -e 127 'popd built-in is unavailable in POSIX mode' --posix
echo echo not reached > popd
chmod a+x popd
PATH=$PWD:$PATH
popd --help
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
