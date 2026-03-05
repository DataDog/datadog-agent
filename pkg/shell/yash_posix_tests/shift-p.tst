# shift-p.tst: test of the shift built-in for any POSIX-compliant shell

posix="true"

setup -d

test_oE -e 0 'shift 0 -> 0' -es
shift 0 && bracket "$#" "$@"
__IN__
[0]
__OUT__

test_oE -e 0 'shift 1 -> 1' -es a
shift 0 && bracket "$#" "$@"
__IN__
[1][a]
__OUT__

test_oE -e 0 'shift 1 -> 0' -es a
shift 1 && bracket "$#" "$@"
__IN__
[0]
__OUT__

test_oE -e 0 'shift 2 -> 2' -es a 'b  b'
shift 0 && bracket "$#" "$@"
__IN__
[2][a][b  b]
__OUT__

test_oE -e 0 'shift 2 -> 1' -es a 'b  b'
shift 1 && bracket "$#" "$@"
__IN__
[1][b  b]
__OUT__

test_oE -e 0 'shift 2 -> 0' -es a 'b  b'
shift 2 && bracket "$#" "$@"
__IN__
[0]
__OUT__

test_oE -e 0 'shift 10 -> 3' -es a 'b  b' c d e f g '' - j
shift 7 && bracket "$#" "$@"
__IN__
[3][][-][j]
__OUT__

test_O -d -e n 'too large operand 1 for 0' -es
shift 1
__IN__

test_O -d -e n 'too large operand 2 for 1' -es a
shift 2
__IN__

test_O -d -e n 'too large operand 3 for 2' -es a 'b  b'
shift 3
__IN__

test_O -d -e n 'too large operand 100 for 10' -es a 'b  b' c d e f g '' - j
shift 100
__IN__

test_oE -e 0 'default operand is 1: success' -es a 'b  b' c
shift && bracket "$#" "$@"
shift && bracket "$#" "$@"
shift && bracket "$#" "$@"
__IN__
[2][b  b][c]
[1][c]
[0]
__OUT__

test_O -d -e n 'default operand is 1: failure' -es
shift
__IN__

test_oE -e 0 'arguments are shifted in function' -es a 'b  b' c
func() { shift; bracket "$#" "$@"; }
func x 'y  y' z
bracket "$#" "$@"
__IN__
[2][y  y][z]
[3][a][b  b][c]
__OUT__

test_oE -e 0 'separator preceding operand' -es a b c d e
shift -- 2 && bracket "$#" "$@"
__IN__
[3][c][d][e]
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
