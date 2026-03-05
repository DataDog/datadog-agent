# cd-p.tst: test of the cd built-in for any POSIX-compliant shell

# Tests in this file may fail if the pathname of the current directory is too
# long, making the pathname of temporary directories exceed PATH_MAX.

posix="true"

cd -P .
export ORIGPWD="$PWD"
mkdir -p cdpath1/foo cdpath2/foo/bar cdpath2/dev dev
mkdir -m 400 no_search_dir
ln -s cdpath2/foo link
>file

test_oE 'default operand is HOME (-L)'
HOME=/dev
cd -L
echo --- $?
pwd
__IN__
--- 0
/dev
__OUT__

test_oE 'default operand is HOME (-P)'
HOME=/dev
cd -P
echo --- $?
pwd
__IN__
--- 0
/dev
__OUT__

(
# Ensure $PWD is safe to assign to $PATH
case $PWD in (*[:%]*)
    skip="true"
esac

testcase "$LINENO" 'found in first cd path (-L)' \
    3<<\__IN__ 5</dev/null 4<<__OUT__
CDPATH=$ORIGPWD/cdpath1::$ORIGPWD/cdpath2
cd -L foo
echo --- $?
pwd
__IN__
$ORIGPWD/cdpath1/foo
--- 0
$ORIGPWD/cdpath1/foo
__OUT__

testcase "$LINENO" 'found in first cd path (-P)' \
    3<<\__IN__ 5</dev/null 4<<__OUT__
CDPATH=$ORIGPWD/cdpath1::$ORIGPWD/cdpath2
cd -P foo
echo --- $?
pwd
__IN__
$ORIGPWD/cdpath1/foo
--- 0
$ORIGPWD/cdpath1/foo
__OUT__

testcase "$LINENO" 'found in last cd path (-L)' \
    3<<\__IN__ 5</dev/null 4<<__OUT__
CDPATH=$ORIGPWD/cdpath1::$ORIGPWD/cdpath2
cd -L foo/bar
echo --- $?
pwd
__IN__
$ORIGPWD/cdpath2/foo/bar
--- 0
$ORIGPWD/cdpath2/foo/bar
__OUT__

testcase "$LINENO" 'found in last cd path (-P)' \
    3<<\__IN__ 5</dev/null 4<<__OUT__
CDPATH=$ORIGPWD/cdpath1::$ORIGPWD/cdpath2
cd -P foo/bar
echo --- $?
pwd
__IN__
$ORIGPWD/cdpath2/foo/bar
--- 0
$ORIGPWD/cdpath2/foo/bar
__OUT__

testcase "$LINENO" 'found in empty cd path (-L)' \
    3<<\__IN__ 5</dev/null 4<<__OUT__
CDPATH=$ORIGPWD/cdpath1::$ORIGPWD/cdpath2
cd -L dev
echo --- $?
pwd
__IN__
--- 0
$ORIGPWD/dev
__OUT__

testcase "$LINENO" 'found in empty cd path (-P)' \
    3<<\__IN__ 5</dev/null 4<<__OUT__
CDPATH=$ORIGPWD/cdpath1::$ORIGPWD/cdpath2
cd -P dev
echo --- $?
pwd
__IN__
--- 0
$ORIGPWD/dev
__OUT__

testcase "$LINENO" 'found in dot cd path (-L)' \
    3<<\__IN__ 5</dev/null 4<<__OUT__
CDPATH=$ORIGPWD/cdpath1:.:$ORIGPWD/cdpath2
cd -L dev
echo --- $?
pwd
__IN__
$ORIGPWD/dev
--- 0
$ORIGPWD/dev
__OUT__

testcase "$LINENO" 'found in dot cd path (-P)' \
    3<<\__IN__ 5</dev/null 4<<__OUT__
CDPATH=$ORIGPWD/cdpath1:.:$ORIGPWD/cdpath2
cd -P dev
echo --- $?
pwd
__IN__
$ORIGPWD/dev
--- 0
$ORIGPWD/dev
__OUT__

test_oE 'cd path ending with slash (-L)'
CDPATH=/
cd -L dev
echo --- $?
pwd
__IN__
/dev
--- 0
/dev
__OUT__

test_oE 'cd path ending with slash (-P)'
CDPATH=/
cd -P dev
echo --- $?
pwd
__IN__
/dev
--- 0
/dev
__OUT__

testcase "$LINENO" 'found not in any cd path, but in PWD (-L)' \
    3<<\__IN__ 5</dev/null 4<<__OUT__
CDPATH=$ORIGPWD/cdpath1:$ORIGPWD/cdpath2
cd -L cdpath1
echo --- $?
pwd
__IN__
--- 0
$ORIGPWD/cdpath1
__OUT__

testcase "$LINENO" 'found not in any cd path, but in PWD (-P)' \
    3<<\__IN__ 5</dev/null 4<<__OUT__
CDPATH=$ORIGPWD/cdpath1:$ORIGPWD/cdpath2
cd -P cdpath1
echo --- $?
pwd
__IN__
--- 0
$ORIGPWD/cdpath1
__OUT__

test_oE 'cd paths are ignored for absolute path operand (-L)'
CDPATH=$ORIGPWD/cdpath1::$ORIGPWD/cdpath2
cd -L /dev
echo --- $?
pwd
__IN__
--- 0
/dev
__OUT__

test_oE 'cd paths are ignored for absolute path operand (-P)'
CDPATH=$ORIGPWD/cdpath1::$ORIGPWD/cdpath2
cd -P /dev
echo --- $?
pwd
__IN__
--- 0
/dev
__OUT__

testcase "$LINENO" 'cd paths are ignored for operand starting with dot (-L)' \
    3<<\__IN__ 5</dev/null 4<<__OUT__
CDPATH=$ORIGPWD/cdpath2
cd -L ./dev
echo --- $?
pwd
__IN__
--- 0
$ORIGPWD/dev
__OUT__

testcase "$LINENO" 'cd paths are ignored for operand starting with dot (-P)' \
    3<<\__IN__ 5</dev/null 4<<__OUT__
CDPATH=$ORIGPWD/cdpath2
cd -P ./dev
echo --- $?
pwd
__IN__
--- 0
$ORIGPWD/dev
__OUT__

testcase "$LINENO" 'cd paths are ignored for operand starting with dot-dot (-L)' \
    3<<\__IN__ 5</dev/null 4<<__OUT__
unset CDPATH
cd -L cdpath1
CDPATH=$ORIGPWD/cdpath2
cd ../dev
echo --- $?
pwd
__IN__
--- 0
$ORIGPWD/dev
__OUT__

testcase "$LINENO" 'cd paths are ignored for operand starting with dot-dot (-P)' \
    3<<\__IN__ 5</dev/null 4<<__OUT__
unset CDPATH
cd -P cdpath1
CDPATH=$ORIGPWD/cdpath2
cd ../dev
echo --- $?
pwd
__IN__
--- 0
$ORIGPWD/dev
__OUT__

testcase "$LINENO" -d 'not found in any cd path nor in PWD (-L)' \
    3<<\__IN__ 5<&- 4<<__OUT__
CDPATH=$ORIGPWD/cdpath1::$ORIGPWD/cdpath2
cd -L _no_such_path_
echo --- $((!$?))
pwd
__IN__
--- 0
$ORIGPWD
__OUT__

testcase "$LINENO" -d 'not found in any cd path nor in PWD (-P)' \
    3<<\__IN__ 5<&- 4<<__OUT__
CDPATH=$ORIGPWD/cdpath1::$ORIGPWD/cdpath2
cd -P _no_such_path_
echo --- $((!$?))
pwd
__IN__
--- 0
$ORIGPWD
__OUT__

)

testcase "$LINENO" -d 'directory not found (with unset CDPATH, -L)' \
    3<<\__IN__ 5<&- 4<<__OUT__
unset CDPATH
cd -L _no_such_path_
echo --- $((!$?))
pwd
__IN__
--- 0
$ORIGPWD
__OUT__

testcase "$LINENO" -d 'directory not found (with unset CDPATH, -P)' \
    3<<\__IN__ 5<&- 4<<__OUT__
unset CDPATH
cd -P _no_such_path_
echo --- $((!$?))
pwd
__IN__
--- 0
$ORIGPWD
__OUT__

test_O -d -e n 'non-directory file in operand component (-L)'
cd -L ./file/../dev
__IN__

test_O -d -e n 'non-directory file in operand component (-P)'
cd -P ./file/../dev
__IN__

test_O -d -e n 'non-existing file in operand component (-L)'
cd -L ./_no_such_file_/../dev
__IN__

test_O -d -e n 'non-existing file in operand component (-P)'
cd -P ./_no_such_file_/../dev
__IN__

testcase "$LINENO" 'target pathname is canonicalized (-L)' \
    3<<\__IN__ 5<&- 4<<__OUT__
unset CDPATH
cd -L link/./../dev/.
printf 'PWD=%s\n' "$PWD"
pwd
__IN__
PWD=$ORIGPWD/dev
$ORIGPWD/dev
__OUT__

testcase "$LINENO" 'symbolic links are resolved (in operand, -P)' \
    3<<\__IN__ 5<&- 4<<__OUT__
unset CDPATH
cd -P link/./../dev/.
printf 'PWD=%s\n' "$PWD"
pwd
__IN__
PWD=$ORIGPWD/cdpath2/dev
$ORIGPWD/cdpath2/dev
__OUT__

testcase "$LINENO" 'symbolic links are resolved (in old PWD, -P)' \
    3<<\__IN__ 5<&- 4<<__OUT__
unset CDPATH
cd -L link
cd -P ./../dev/.
printf 'PWD=%s\n' "$PWD"
pwd
__IN__
PWD=$ORIGPWD/cdpath2/dev
$ORIGPWD/cdpath2/dev
__OUT__

testcase "$LINENO" 'default option is -L' \
    3<<\__IN__ 5<&- 4<<__OUT__
unset CDPATH
cd link/./../dev/.
printf 'PWD=%s\n' "$PWD"
pwd
__IN__
PWD=$ORIGPWD/dev
$ORIGPWD/dev
__OUT__

testcase "$LINENO" 'the last option wins (-L)' \
    3<<\__IN__ 5<&- 4<<__OUT__
unset CDPATH
cd -P -L -PL link/./../dev/.
printf 'PWD=%s\n' "$PWD"
pwd
__IN__
PWD=$ORIGPWD/dev
$ORIGPWD/dev
__OUT__

testcase "$LINENO" 'the last option wins (-P)' \
    3<<\__IN__ 5<&- 4<<__OUT__
unset CDPATH
cd -L -P -LP link/./../dev/.
printf 'PWD=%s\n' "$PWD"
pwd
__IN__
PWD=$ORIGPWD/cdpath2/dev
$ORIGPWD/cdpath2/dev
__OUT__

test_x -e 0 'exit status of success with -e'
cd -P -e .
__IN__

# There is no reliable way to test this case.
#test_x -e 1 'exit status of failure with -e'

test_x -e 0 'exit status of change error with -e'
cd -P -e _no_such_path_
[ $? -gt 1 ]
__IN__

(
# Skip if we're root.
if [ -d no_search_dir/. ]; then
    skip="true"
fi

test_O -d -e n 'changing to unsearchable directory (-L)'
cd -L no_search_dir
__IN__

test_O -d -e n 'changing to unsearchable directory (-P)'
cd -P no_search_dir
__IN__

)

test_oE 'hyphen operand means OLDPWD (-L)'
OLDPWD=/dev
cd -L -
echo --- $?
pwd
__IN__
/dev
--- 0
/dev
__OUT__

test_oE 'hyphen operand means OLDPWD (-P)'
OLDPWD=/dev
cd -P -
echo --- $?
pwd
__IN__
/dev
--- 0
/dev
__OUT__

testcase "$LINENO" 'OLDPWD is set to old PWD (-L)' \
    3<<\__IN__ 5<&- 4<<__OUT__
unset CDPATH
cd -L /
printf 'OLDPWD=%s\n' "$OLDPWD"
__IN__
OLDPWD=$ORIGPWD
__OUT__

test_O -d -e n 'empty operand (-L)'
cd -L ''
__IN__

test_O -d -e n 'empty operand (-P)'
cd -P ''
__IN__

test_O -d -e n 'readonly PWD (-L)'
# As specified in POSIX XBD 8.1, one of the following should happen:
# - The readonly built-in fails.
# - The cd built-in fails.
# - The cd built-in succeeds as if the readonly built-in had not been executed.
readonly PWD && cd -L / &&
if [ "$PWD" = / ]; then
    printf 'PWD successfully changed\n' >&2
    false # The expected exit status of this test is non-zero.
fi
__IN__

test_O -d -e n 'readonly PWD (-P)'
# As specified in POSIX XBD 8.1, one of the following should happen:
# - The readonly built-in fails.
# - The cd built-in fails.
# - The cd built-in succeeds as if the readonly built-in had not been executed.
readonly PWD && cd -P / &&
if [ "$PWD" = / ]; then
    printf 'PWD successfully changed\n' >&2
    false # The expected exit status of this test is non-zero.
fi
__IN__

test_x -d -e n 'readonly OLDPWD (-L)'
# As specified in POSIX XBD 8.1, one of the following should happen:
# - The readonly built-in fails.
# - The cd built-in fails.
# - The cd built-in succeeds as if the readonly built-in had not been executed.
cd /
readonly OLDPWD && cd -L - &&
if [ "$OLDPWD" = / ]; then
    printf 'OLDPWD successfully changed\n' >&2
    false # The expected exit status of this test is non-zero.
fi
__IN__

test_x -d -e n 'readonly OLDPWD (-P)'
# As specified in POSIX XBD 8.1, one of the following should happen:
# - The readonly built-in fails.
# - The cd built-in fails.
# - The cd built-in succeeds as if the readonly built-in had not been executed.
cd /
readonly OLDPWD && cd -P - &&
if [ "$OLDPWD" = / ]; then
    printf 'OLDPWD successfully changed\n' >&2
    false # The expected exit status of this test is non-zero.
fi
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
