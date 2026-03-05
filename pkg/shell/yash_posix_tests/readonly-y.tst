# readonly-y.tst: yash-specific test of the readonly built-in

(
setup 'readonly a=foo b=bar c=baz'

test_oE -e 0 'printing all read-only variables'
readonly -p
__IN__
readonly a=foo
readonly b=bar
readonly c=baz
__OUT__

test_oE -e 0 'printing specific read-only variables'
readonly -p a b
__IN__
readonly a=foo
readonly b=bar
__OUT__

test_OE -e 0 'without argument, -p is assumed (variables)'
readonly >withoutp.out
readonly -p >withp.out
diff withoutp.out withp.out
__IN__

)

test_oE -e 0 'assigning empty value'
readonly a=
readonly -p a
__IN__
readonly a=''
__OUT__

test_o -d 'reusing printed read-only variables'
eval ":; $(
    readonly a=A
    readonly -p
)"
echo 1 $a
a=X && echo not reached
__IN__
1 A
__OUT__

test_oE -e 0 'separator preceding scalar variable name starting with -' -e
readonly -- -a=1
readonly -p -- -a
__IN__
readonly -- -a=1
__OUT__

test_oE -e 0 'separator preceding function name starting with -' -e
function -n() { :; }
readonly -f -- -n
readonly -fp -- -n
__IN__
function -n()
{
   :
}
readonly -f -- -n
__OUT__

test_oE 'making read-only with -p (variables)'
readonly -p a=A
readonly -p a
__IN__
readonly a=A
__OUT__

test_Oe -e 1 'assigning to read-only variable'
readonly a=A
readonly a=X
__IN__
readonly: $a is read-only
__ERR__

test_OE -e 0 'assigning to variable with empty name'
readonly =X # This succeeds, but the variable can never be used.
__IN__

test_Oe -e 1 'making array read-only' -e
a=(1)
readonly a
readonly a=1
__IN__
readonly: $a is read-only
__ERR__

testcase "$LINENO" 'making function read-only' \
    3<<\__IN__ 4<<\__OUT__ 5<<__ERR__
foo() { echo foo 1; }
bar() { echo bar 1; }
readonly -f bar
foo() { echo foo 2; }
bar() { echo bar 2; }
echo $?
foo
bar
__IN__
2
foo 2
bar 1
__OUT__
$testee: function \`bar' cannot be redefined because it is read-only
__ERR__

(
setup 'foo() { echo foo; }; bar() { echo bar; }; baz() { echo baz; }'
setup 'readonly -f foo bar'

test_x -e 0 'printing all read-only functions: exit status'
readonly -fp
__IN__

test_oE 'printing all read-only functions: output'
readonly -fp | sed 's;^[[:space:]]*;;g'
__IN__
bar()
{
echo bar
}
readonly -f bar
foo()
{
echo foo
}
readonly -f foo
__OUT__

test_x -e 0 'printing specific read-only functions: exit status'
readonly -fp bar
__IN__

test_oE 'printing specific read-only functions: output'
readonly -fp bar | sed 's;^[[:space:]]*;;g'
__IN__
bar()
{
echo bar
}
readonly -f bar
__OUT__

test_OE -e 0 'without argument, -p is assumed (functions)'
readonly -f >withoutp.out
readonly -f -p >withp.out
diff withoutp.out withp.out
__IN__

)

test_o -d 'reusing printed read-only functions'
eval "$(
    func() { echo func; }
    readonly -f func
    readonly -fp func
)"
func
func() { :; }
func
__IN__
func
func
__OUT__

(
posix="true"

test_Oe -e 2 'invalid option -f (POSIX)'
readonly -f
__IN__
readonly: `-f' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'invalid option -x (POSIX)'
readonly -x
__IN__
readonly: `-x' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'invalid option -X (POSIX)'
readonly -X
__IN__
readonly: `-X' is not a valid option
__ERR__
#'
#`

test_Oe -e 1 'LINENO cannot be read-only (POSIX)'
readonly LINENO
__IN__
readonly: $LINENO cannot be made read-only in the POSIXly-correct mode
__ERR__

test_Oe -e 1 'OPTARG cannot be read-only (POSIX)'
readonly OPTARG
__IN__
readonly: $OPTARG cannot be made read-only in the POSIXly-correct mode
__ERR__

test_Oe -e 1 'OPTIND cannot be read-only (POSIX)'
readonly OPTIND
__IN__
readonly: $OPTIND cannot be made read-only in the POSIXly-correct mode
__ERR__

test_Oe -e 1 'PWD cannot be read-only (POSIX)'
readonly PWD
__IN__
readonly: $PWD cannot be made read-only in the POSIXly-correct mode
__ERR__

test_Oe -e 1 'OLDPWD cannot be read-only (POSIX)'
readonly OLDPWD
__IN__
readonly: $OLDPWD cannot be made read-only in the POSIXly-correct mode
__ERR__

)

test_Oe -e 2 'invalid option -z'
readonly -z
__IN__
readonly: `-z' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'invalid option --xxx'
readonly --no-such=option
__IN__
readonly: `--no-such=option' is not a valid option
__ERR__
#'
#`

test_O -d -e 1 'printing to closed stream'
readonly a=X
readonly >&-
__IN__

test_Oe -e 1 'printing non-existing variable'
unset a
readonly -p a
__IN__
readonly: no such variable $a
__ERR__

test_Oe -e 1 'printing non-existing function'
readonly -fp a
__IN__
readonly: no such function `a'
__ERR__
#'
#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
