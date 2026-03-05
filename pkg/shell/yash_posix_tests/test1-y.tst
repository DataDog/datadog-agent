# test1-y.tst: yash-specific test of the test built-in, part 1

if ! testee -c 'command -bv test' >/dev/null; then
    skip="true"
fi

. ../test-y.sh

umask u=rwx,go=

>file
ln -s file filelink
ln -s _no_such_file_ brokenlink

touch -a -t 200101010000 old; touch -m -t 200001010000 old
touch -a -t 200001010000 new; touch -m -t 200101010000 new

(
mkdir dir sticky
if ! { chmod a-t dir && chmod a+t sticky; } then
    skip="true"
fi
ln -s sticky stickylink

assert_true -k
assert_true -k sticky
assert_true -k stickylink
assert_false -k file
assert_false -k filelink
assert_false -k dir
assert_false -k dirlink
assert_false -k ./_no_such_file_
assert_false -k brokenlink
)

assert_true -G
# Tests for the -G operator is missing
assert_false -G ./_no_such_file_
assert_false -G brokenlink

assert_true -N
assert_true -N new
assert_false -N old
assert_false -N ./_no_such_file_
assert_false -N brokenlink

assert_true -O
# Tests for the -O operator is missing
assert_false -O ./_no_such_file_
assert_false -O brokenlink

assert_true -o
assert_false -o allexpo
assert_false -o allexport
assert_false -o all-_export
assert_false -o allexportttttttt
assert_true -o \?allexpo
assert_true -o \?allexport
assert_true -o \?all-_export
assert_false -o \?allexportttttttt
(
setup 'set -o allexport'
assert_true -o allexpo
assert_true -o allexport
assert_true -o all-_export
assert_false -o allexportttttttt
assert_true -o \?allexpo
assert_true -o \?allexport
assert_true -o \?all-_export
assert_false -o \?allexportttttttt
)

assert_false -o tify
assert_false -o notify
assert_true -o nonotify
assert_true -o n-o-n-otify
assert_false -o \?tify
assert_true -o \?notify
assert_true -o \?nonotify
assert_true -o \?n-o-n-otify
(
setup 'set -o notify'
assert_false -o tify
assert_true -o notify
assert_false -o nonotify
assert_false -o n-o-n-otify
assert_false -o \?tify
assert_true -o \?notify
assert_true -o \?nonotify
assert_true -o \?n-o-n-otify
)

assert_false "" -a ""
assert_false "" -a 1
assert_false 1 -a ""
assert_true 1 -a 1

assert_false "" -o ""
assert_true "" -o 1
assert_true 1 -o ""
assert_true 1 -o 1

assert_true "" -a 1 -o 1  # -a has higher precedence than -o
assert_true 1 -o 1 -a ""  # -a has higher precedence than -o
assert_true ! "" -a ""    # ! ( "" -a "" )
assert_true ! 0 -a ""     # ! ( 0 -a "" )

assert_false "(" "" ")"
assert_true "(" A ")"
assert_true "(" xyz ")"
assert_true "(" 12345 = 12345 ")"
assert_false "(" 12345 = abcde ")"
assert_true "(" "(" 12345 = 12345 ")" ")"
assert_false "(" "(" 12345 = abcde ")" ")"
assert_false "(" ! a = a ")"
assert_true "(" ! a = b ")"

assert_true 1 -a "(" 1 = 0 -o "(" 2 = 2 ")" ")" -a "(" = ")"
assert_true -n = -o -o -n = -n  # ( -n = -o ) -o ( -n = -n )
assert_true -n = -a -n = -n     # ( -n = ) -a ( -n = -n )

test_Oe -e 2 'invalid unary operator'
test 1 2
__IN__
test: `1' is not a unary operator
__ERR__
#'
#`

test_Oe -e 2 'invalid binary operator'
test 1 2 3
__IN__
test: `2' is not a binary operator
__ERR__
#'
#`

(
posix=true

test_Oe -e 2 'parentheses not supported in POSIX mode, single'
test "(" foo ")"
__IN__
test: parentheses cannot be used in the POSIXly-correct mode
__ERR__

test_Oe -e 2 'parentheses not supported in POSIX mode, double'
test "(" -n foo ")"
__IN__
test: parentheses cannot be used in the POSIXly-correct mode
__ERR__

test_Oe -e 2 'parentheses not supported in POSIX mode, long'
test "(" foo = foo ")"
__IN__
test: parentheses cannot be used in the POSIXly-correct mode
__ERR__

test_Oe -e 2 'binary -a not supported in POSIX mode, single'
test 1 -a 2
__IN__
test: binary operator `-a' cannot be used in the POSIXly-correct mode
__ERR__
#'
#`

test_Oe -e 2 'binary -a not supported in POSIX mode, long'
test 1 -eq 1 -a 2 -eq 2
__IN__
test: binary operator `-a' cannot be used in the POSIXly-correct mode
__ERR__
#'
#`

test_Oe -e 2 'binary -o not supported in POSIX mode, single'
test 1 -o 2
__IN__
test: binary operator `-o' cannot be used in the POSIXly-correct mode
__ERR__
#'
#`

test_Oe -e 2 'binary -o not supported in POSIX mode, long'
test 1 -eq 1 -o 2 -eq 2
__IN__
test: binary operator `-o' cannot be used in the POSIXly-correct mode
__ERR__
#'
#`

)

# vim: set ft=sh ts=8 sts=4 sw=4 et:
