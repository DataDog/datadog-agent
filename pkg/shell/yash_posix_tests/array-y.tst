# array-y.tst: yash-specific test of arrays

setup -d
setup - <<\__END__
set -e
__END__

test_oE -e 0 'assigning empty array'
e=()
bracket "$e"
__IN__

__OUT__

test_oE -e 0 'assigning array with empty expansion'
e=($_empty)
bracket "$e"
__IN__

__OUT__

test_oE -e 0 'assigning non-empty array'
set 1 '2  2' 3
a='4  5'
a=("$@" $a)
bracket "$a"
__IN__
[1][2  2][3][4][5]
__OUT__

test_oE -e 0 'multi-line array assignment'
set 1 '2  2' 3
a='4  5'
a=(
"$@"
    
$a
) b=b c=(c)
bracket "$a" "$b" "$c"
__IN__
[1][2  2][3][4][5][b][c]
__OUT__

test_oE -e 0 'array value containing parentheses'
a=(\)\()
bracket "$a"
__IN__
[)(]
__OUT__

test_oE -e 0 'comment in array value'
a=A
b=(
$a # 1
# 2
B
)
bracket "$b"
__IN__
[A][B]
__OUT__

test_Oe -e 2 'unclosed array assignment'
a=(
__IN__
syntax error: `)' is missing
__ERR__
#'
#`

test_Oe -e 2 'unquoted symbol in array assignment'
a=(1;)
__IN__
syntax error: `)' is missing
__ERR__
#'
#`

test_oE -e 0 'reassigning array'
a=(a)
a=(b c)
bracket "$a"
__IN__
[b][c]
__OUT__

test_oE -e 0 'reassigning export array'
export a
a=(a)
a=(b c)
sh -c 'echo "$a"'
__IN__
b:c
__OUT__

# Below are tests of the array built-in.
if ! testee --version --verbose | grep -Fqx ' * array'; then
    skip="true"
fi

test_OE -e 0 'printing all arrays (none defined)'
array
__IN__

(
setup - <<\__END__
a=(1 '2  2' 3)
e=()
b=(ABC)
__END__

test_oE -e 0 'printing all arrays (some defined)'
array
__IN__
a=(1 '2  2' 3)
b=(ABC)
e=()
__OUT__

test_O -d -e n 'printing all arrays to closed stream'
array >&-
__IN__

test_oE -e 0 'defining array'
array b
array c x 'y  y' z
bracket "$b"
bracket "$c"
__IN__

[x][y  y][z]
__OUT__

test_oE -e 0 'defining array (exported)'
export a e
sh -c 'echo "$a"'
sh -c 'echo "$e"'
array a b c
sh -c 'echo "$a"'
__IN__
1:2  2:3

b:c
__OUT__

test_Oe -e n 'defining array (overwriting read-only array)'
readonly a
array a A B C
__IN__
array: $a is read-only
__ERR__

test_Oe -e n 'defining array (invalid name)'
array a= A B C
__IN__
array: `a=' is not a valid array name
__ERR__
#'
#`

(
setup 'c=(1 2 3 4 5 6 7 8 9 10)'

test_oE -e 0 'deleting array elements'
array -d a
array -d b 1
array --delete -- c 2 5 -2 11 -100
array --delete -- e -1 0 1
array
__IN__
a=(1 '2  2' 3)
b=()
c=(1 3 4 6 7 8 10)
e=()
__OUT__

test_oE -e 0 'deleting array elements (index ordering)'
array -d -- c -2 11 2 -100 5 11
bracket "$c"
__IN__
[1][3][4][6][7][8][10]
__OUT__

test_oE -e 0 'deleting array elements (duplicate indices)'
array -d -- c 2 8 6 6 8 2 -3
bracket "$c"
__IN__
[1][3][4][5][7][9][10]
__OUT__

test_oE -e 0 'deleting array elements (border cases)'
array -d c 0 11 -11
bracket "$c"
array -d c 1 -1
bracket "$c"
array -d c 8 -8
bracket "$c"
__IN__
[1][2][3][4][5][6][7][8][9][10]
[2][3][4][5][6][7][8][9]
[3][4][5][6][7][8]
__OUT__

test_oE -e 0 'deleting array elements (exported)'
export c
array -d c 2 5 -2 11 -100
sh -c 'echo "$c"'
__IN__
1:3:4:6:7:8:10
__OUT__

)

test_Oe -e n 'deleting array elements (nonexistent array)'
array -d x
__IN__
array: no such array $x
__ERR__

test_Oe -e n 'deleting array elements (read-only array)'
readonly a
array -d a 1
__IN__
array: $a is read-only
__ERR__

test_Oe -e n 'deleting array elements (invalid name)'
array -d = 1
__IN__
array: `=' is not a valid array name
__ERR__
#'
#`

test_Oe -e n 'deleting array elements (missing operand)'
array -d
__IN__
array: this command requires an operand
__ERR__

test_oE -e 0 'inserting array elements (middle, none)'
array -i a 2
bracket "$a"
__IN__
[1][2  2][3]
__OUT__

test_oE -e 0 'inserting array elements (middle, one)'
array --insert a 2 I
bracket "$a"
__IN__
[1][2  2][I][3]
__OUT__

test_oE -e 0 'inserting array elements (middle, some)'
array -i a 2 I J K
bracket "$a"
__IN__
[1][2  2][I][J][K][3]
__OUT__

test_oE -e 0 'inserting array elements (head, positive)'
array -i a 0 I J
bracket "$a"
__IN__
[I][J][1][2  2][3]
__OUT__

test_oE -e 0 'inserting array elements (tail, positive)'
array -i a 3 I J
bracket "$a"
__IN__
[1][2  2][3][I][J]
__OUT__

test_oE -e 0 'inserting array elements (over-tail, positive)'
array -i a 4 I J
bracket "$a"
__IN__
[1][2  2][3][I][J]
__OUT__

test_oE -e 0 'inserting array elements (tail, negative)'
array -i -- a -1 I J
bracket "$a"
__IN__
[1][2  2][3][I][J]
__OUT__

test_oE -e 0 'inserting array elements (head, negative)'
array -i -- a -4 I J
bracket "$a"
__IN__
[I][J][1][2  2][3]
__OUT__

test_oE -e 0 'inserting array elements (exported)'
export a
array -i a 2 I J
sh -c 'echo "$a"'
__IN__
1:2  2:I:J:3
__OUT__

test_oE -e 0 'inserting array elements (over-head, negative)'
array -i -- a -5 I J
bracket "$a"
__IN__
[I][J][1][2  2][3]
__OUT__

test_Oe -e n 'inserting array elements (nonexistent array)'
array -i x 1 ''
__IN__
array: no such array $x
__ERR__

test_Oe -e n 'inserting array elements (read-only array)'
readonly a
array -i a 1 A
__IN__
array: $a is read-only
__ERR__

test_Oe -e n 'inserting array elements (invalid name)'
array -i = 1 A
__IN__
array: `=' is not a valid array name
__ERR__
#'
#`

test_Oe -e n 'inserting array elements (missing operand)'
array -i a
__IN__
array: this command requires 2 operands
__ERR__

test_oE -e 0 'setting array element (head, positive)'
array -s a 1 A
bracket "$a"
__IN__
[A][2  2][3]
__OUT__

test_oE -e 0 'setting array element (tail, positive)'
array --set a 3 C
bracket "$a"
__IN__
[1][2  2][C]
__OUT__

test_Oe -e n 'setting array element (over-tail, positive)'
array -s a 4 C
__IN__
array: index 4 is out of range (the actual size of array $a is 3)
__ERR__

test_Oe -e n 'setting array element (index zero)'
array -s a 0 C
__IN__
array: index 0 is out of range (the actual size of array $a is 3)
__ERR__

test_oE -e 0 'setting array element (tail, negative)'
array -s a -1 C
bracket "$a"
__IN__
[1][2  2][C]
__OUT__

test_oE -e 0 'setting array element (head, negative)'
array -s a -3 A
bracket "$a"
__IN__
[A][2  2][3]
__OUT__

test_oE -e 0 'setting array element (exported)'
export a
array -s a 1 A
sh -c 'echo "$a"'
__IN__
A:2  2:3
__OUT__

test_Oe -e n 'setting array element (over-head, negative)'
array -s a -4 C
__IN__
array: index -4 is out of range (the actual size of array $a is 3)
__ERR__

test_Oe -e n 'setting array element (empty array)'
array -s e 1 ''
__IN__
array: index 1 is out of range (the actual size of array $e is 0)
__ERR__

test_Oe -e n 'setting array element (nonexistent array)'
array -s x 1 ''
__IN__
array: no such array $x
__ERR__

test_Oe -e n 'setting array element (read-only array)'
readonly a
array -s a 1 A
__IN__
array: $a is read-only
__ERR__

test_Oe -e n 'setting array element (invalid name)'
array -s = 1 A
__IN__
array: `=' is not a valid array name
__ERR__
#'
#`

test_Oe -e n 'setting array element (too many operands)'
array -s a 1 A B
__IN__
array: too many operands are specified
__ERR__

test_Oe -e n 'setting array element (too few operands)'
array -s a 1
__IN__
array: this command requires 3 operands
__ERR__

)

test_Oe -e n 'invalid option'
array --no-such-option
__IN__
array: `--no-such-option' is not a valid option
__ERR__
#'
#`

test_Oe -e 2 'no array assignment in POSIX mode' --posix
foo=()
__IN__
syntax error: invalid use of `('
__ERR__
#'
#`

test_oE -e 0 'array built-in is unavailable in POSIX mode: w/ external' --posix
mkdir cmdtmp
cd cmdtmp
echo echo external script executed > array
chmod a+x array
PATH=$PWD:$PATH
array --help
__IN__
external script executed
__OUT__

test_Oe -e 127 'array built-in is unavailable in POSIX mode: w/o external' \
    --posix
PATH=
eval 'array --help'
__IN__
eval: no such command `array'
__ERR__
#'
#`

# vim: set ft=sh ts=8 sts=4 sw=4 et:
