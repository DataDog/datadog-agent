# dot-y.tst: yash-specific test of the dot built-in

echo true >true
echo 'echo "$*"' >print_args
echo 'set a "$@"; echo "$*"' >set_print_args

test_oE 'positional parameters in dot script'
set 1 2 3
. ./print_args
__IN__
1 2 3
__OUT__

test_oE 'changing outer-scope positional parameters in dot script'
set 1 2 3
. ./set_print_args
echo "$*"
__IN__
a 1 2 3
a 1 2 3
__OUT__

test_oE 'temporary positional parameters'
set 1 2 3
. ./print_args x y
echo "$*"
__IN__
x y
1 2 3
__OUT__

test_oE 'changing temporary positional parameters'
set 1 2 3
. ./set_print_args x y
echo "$*"
__IN__
a x y
1 2 3
__OUT__

test_o 'positional parameters removed on error'
set foo
command . ./_no_such_file_ bar
echo "$*"
__IN__
foo
__OUT__

(
setup 'alias true=false'

test_OE -e n 'alias in dot script (POSIX)' --posix
. ./true
__IN__

test_OE -e n 'alias in dot script (non-POSIX)'
. ./true
__IN__

test_OE -e 0 'disabling alias (short option)'
. -A ./true
__IN__

test_OE -e 0 'disabling alias (long option)'
. --no-alias ./true
__IN__

)

(
# Ensure $PWD is safe to assign to $PATH/$YASH_LOADPATH
case $PWD in (*[:%]*)
    skip="true"
esac

mkdir testpath
echo 'echo foo' >testpath/foo

mkdir testpath/dir
echo 'echo bar' >testpath/dir/bar

(
setup 'YASH_LOADPATH="$PWD/testpath"'

test_oE 'using $LOADPATH (short option)'
. -L foo
__IN__
foo
__OUT__

test_oE 'using $LOADPATH (long option)'
. --autoload foo
__IN__
foo
__OUT__

test_oE 'dot script in subdirectory of $LOADPATH'
. -L dir/bar
__IN__
bar
__OUT__

)

mkdir testpath/dir1 testpath/dir2

test_oE 'multiple directories in $LOADPATH'
p="$PWD/testpath"
YASH_LOADPATH="$p/dir1:$p/xxx:$p/dir:$p/dir2"
. -L bar
__IN__
bar
__OUT__

(
chmod a+x print_args
if command -v print_args >/dev/null 2>&1; then
    skip="true"
fi
chmod a-x print_args

test_oE -e 0 'dot script not found in $PATH, falling back to $PWD, non-POSIX'
set foo
. print_args
__IN__
foo
__OUT__

)

(
posix=true

test_Oe -e 1 'dot script not found in $PATH, no fallback, POSIX'
PATH=$PWD/_no_such_directory_
set foo
. print_args
__IN__
.: file `print_args' was not found in $PATH
__ERR__
#'
#`

)

)

(
posix='true'

test_Oe -e n 'missing operand'
.
__IN__
.: this command requires an operand
__ERR__

test_Oe -e n 'too many operands'
. _no_such_file_ X
__IN__
.: too many operands are specified
__ERR__

test_Oe -e n 'invalid option'
. -X ''
__IN__
.: `-X' is not a valid option
__ERR__
#'
#`

)

test_Oe -e n 'unset load path'
unset YASH_LOADPATH
. -L _no_such_file_
__IN__
.: file `_no_such_file_' was not found in $YASH_LOADPATH
__ERR__
#'
#`

test_Oe -e n 'null load path'
YASH_LOADPATH=
. -L _no_such_file_
__IN__
.: file `_no_such_file_' was not found in $YASH_LOADPATH
__ERR__
#'
#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
