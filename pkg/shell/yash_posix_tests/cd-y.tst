# cd-y.tst: yash-specific test of the cd built-in

cd -P .
export ORIGPWD="$PWD"
mkdir dir -

test_O -d -e 2 'directory not changeable'
cd _no_such_directory_
__IN__

test_x -e 3 'exit status of non-existing file in operand component (-L)'
cd -L ./_no_such_file_/../dev
__IN__

test_x -e 2 'exit status of non-existing file in operand component (-P)'
cd -P ./_no_such_file_/../dev
__IN__

test_Oe -e 4 'unset HOME'
unset HOME
cd
__IN__
cd: $HOME is not set
__ERR__

test_Oe -e 4 'empty HOME'
HOME=
cd
__IN__
cd: $HOME is not set
__ERR__

test_Oe -e 5 'empty operand'
cd ''
__IN__
cd: empty directory name
__ERR__

testcase "$LINENO" 'unset PWD' \
    3<<\__IN__ 5</dev/null 4<<__OUT__
unset CDPATH PWD
cd dir
echo ---
printf 'PWD=%s\nOLDPWD=%s\n' "$PWD" "$OLDPWD"
pwd
__IN__
---
PWD=$ORIGPWD/dir
OLDPWD=$ORIGPWD
$ORIGPWD/dir
__OUT__

test_Oe -e 4 'unset OLDPWD'
unset OLDPWD
cd -
__IN__
cd: $OLDPWD is not set
__ERR__

testcase "$LINENO" 'read-only PWD' \
    3<<\__IN__ 4<<__OUT__ 5<<\__ERR__
unset CDPATH
readonly PWD
cd dir
echo --- $?
printf 'PWD=%s\n' "$PWD"
pwd
__IN__
--- 1
PWD=$ORIGPWD
$ORIGPWD/dir
__OUT__
cd: $PWD is read-only
__ERR__

test_oe 'unset OLDPWD'
unset CDPATH
readonly OLDPWD=/
cd dir
echo --- $?
printf 'OLDPWD=%s\n' "$OLDPWD"
__IN__
--- 1
OLDPWD=/
__OUT__
cd: $OLDPWD is read-only
__ERR__

test_oE -e 0 'YASH_AFTER_CD is iteratively executed after changing directory'
YASH_AFTER_CD=(
'printf "PWD=%s\n" "$PWD"'
'printf "status=%d\n" "$?"'
'(exit 1); break -i'
'echo not reached')
(exit 11)
cd /
__IN__
PWD=/
status=11
__OUT__

(
posix="true"

test_OE -e 0 'YASH_AFTER_CD is ignored (-o POSIX)'
YASH_AFTER_CD='echo not reached'
cd /
__IN__

)

(
if ! [ / -ef /.. ]; then
    skip="true"
fi

test_oE '/.. is canonicalized to / (+o POSIX)'
cd /..//../dev
printf '%s\n' "$PWD"
cd /../..
printf '%s\n' "$PWD"
__IN__
/dev
/
__OUT__

(
posix="true"

test_oE '/.. is kept intact (-o POSIX)'
cd /../../dev
printf '%s\n' "$PWD"
__IN__
/../../dev
__OUT__

)

)

testcase "$LINENO" 'redundant slashes are removed' \
    3<<\__IN__ 5</dev/null 4<<__OUT__
cd .//dir///
pwd
__IN__
$ORIGPWD/dir
__OUT__

test_oE 'default directory option with operand'
HOME=/tmp cd --default-directory=/ /dev
echo --- $?
pwd
__IN__
--- 0
/dev
__OUT__

test_oE 'default directory option without operand'
HOME=/tmp cd --default-directory=/
echo --- $?
pwd
__IN__
--- 0
/
__OUT__

testcase "$LINENO" 'hyphen is literal in default directory option' \
    3<<\__IN__ 5</dev/null 4<<__OUT__
OLDPWD=/ cd --default-directory=-
pwd
__IN__
$ORIGPWD/-
__OUT__

test_Oe -e 5 'too many operands'
cd . .
__IN__
cd: too many operands are specified
__ERR__

test_Oe -e 5 'invalid option'
cd --no-such-option
__IN__
cd: `--no-such-option' is not a valid option
__ERR__
#'
#`

test_Oe -e 5 -- '-e without -P'
cd -e
__IN__
cd: the -e option requires the -P option
__ERR__

test_O -e 0 'printing to closed stream'
OLDPWD=/ cd - >&-
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
