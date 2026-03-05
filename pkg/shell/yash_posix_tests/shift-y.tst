# shift-y.tst: yash-specific test of the shift built-in

setup -d

(
posix="true"

test_Oe -e 2 'negative operand (POSIX)'
shift -- -1
__IN__
shift: -1: the operand value must not be negative
__ERR__

)

test_oE -e 0 'negative operand (non-POSIX) 1 -> 0' -es a
shift -1 && bracket "$#" "$@"
__IN__
[0]
__OUT__

test_oE -e 0 'negative operand (non-POSIX) 2 -> 1' -es 'a  a' b
shift -1 && bracket "$#" "$@"
__IN__
[1][a  a]
__OUT__

test_oE -e 0 'negative operand (non-POSIX) 2 -> 0' -es 'a  a' b
shift -2 && bracket "$#" "$@"
__IN__
[0]
__OUT__

test_oE -e 0 'array shift 0 -> 0' -e
a=()
shift -A a 0 && bracket "${a[#]}" "$a"
__IN__
[0]
__OUT__

test_oE -e 0 'array shift 10 -> 3' -e
foo=(a 'b  b' c d e f g '' - j)
shift --array=foo -- 7 && bracket "${foo[#]}" "$foo"
__IN__
[3][][-][j]
__OUT__

test_o 'positional parameters are not modified on error' -s a 'b  b' c
shift 4
bracket "$#" "$@"
__IN__
[3][a][b  b][c]
__OUT__

test_o 'array elements are not modified on error'
a=(a 'b  b' c)
shift -A a 4
bracket "${a[#]}" "$a"
__IN__
[3][a][b  b][c]
__OUT__

test_O -d -e 1 'too small operand -1 for 0'
shift -- -1
__IN__

test_O -d -e 1 'too small operand -2 for 1' -s a
shift -- -2
__IN__

test_O -d -e 1 'too large operand 2 for 1 array element'
a=(X)
shift -A a 2
__IN__

test_O -d -e 1 'too large operand 4 for 2 array element'
a=(X Y)
shift -A a 4
__IN__

test_Oe -e 2 'too many operands (against positional parameters)'
shift 1 2
__IN__
shift: too many operands are specified
__ERR__

test_Oe -e 2 'too many operands (against array)'
a=(X Y)
shift -A a 1 2
__IN__
shift: too many operands are specified
__ERR__

test_Oe -e 2 'invalid option -z'
shift -z
__IN__
shift: `-z' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'invalid option --xxx'
shift --no-such=option
__IN__
shift: `--no-such=option' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'missing -A option argument'
shift -A
__IN__
shift: the -A option requires an argument
__ERR__

test_O -d -e 2 'invalid operand (non-numeric)'
shift a
__IN__

test_O -d -e 2 'invalid operand (non-integral)' -s 1
shift 1.0
__IN__

test_Oe -e 1 'invalid array name (containing equal)'
shift -A a=a 0
__IN__
shift: $a=a is not an array
__ERR__

test_Oe -e 1 'invalid array name (non-existing variable)'
shift -A a 0
__IN__
shift: $a is not an array
__ERR__

test_Oe -e 1 'invalid array name (scalar variable)'
a=
shift -A a 0
__IN__
shift: $a is not an array
__ERR__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
