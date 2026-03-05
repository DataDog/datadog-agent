# printf-y.tst: yash-specific test of the printf built-in

test_OE -e 0 'empty operand'
printf ''
__IN__

test_oE -e 0 'newline'
printf '\n'
__IN__

__OUT__

if ! testee -c 'command -bv printf' >/dev/null; then
    skip="true"
fi

test_oE -e 0 'normal characters'
printf \''ABC 123 !"$#&()-=^~|@`[]{}<>;,:.+*/?_'\'
echo
__IN__
'ABC 123 !"$#&()-=^~|@`[]{}<>;,:.+*/?_'
__OUT__

test_oE -e 0 'unformatted operands are ignored'
printf 'ABC\n' ignored operands
__IN__
ABC
__OUT__

test_oE -e 0 'newline 2'
printf 'Multiple\nlines '
printf 'continued\n'
__IN__
Multiple
lines continued
__OUT__

test_oE -e 0 'escaped backslash'
printf '[\\]\n'
__IN__
[\]
__OUT__

test_oE -e 0 'escaped double-quote'
printf '[\"]\n'
__IN__
["]
__OUT__
#"
#'

test_oE -e 0 'escaped single-quote'
printf "[\\']\n"
__IN__
[']
__OUT__
#'
#"

test_oE -e 0 'various escapes'
printf 'a\ab\bf\fr\rt\tv\v-\n'
__IN__
abfrt	v-
__OUT__

test_oE -e 0 'octal numbers'
printf 'octal \11 \1000\n'
__IN__
octal 	 @0
__OUT__

test_oE '%d' -e
printf '%d\n'
printf '%d\n' 0
printf '%d ' 0 10 200 +345 -1278; echo 1
printf '%+d ' 1 20 +345 -1278; echo 2
printf '%+ d ' 1 20 +345 -1278; echo 3
printf '% d ' 1 20 +345 -1278; echo 4
printf '%+8d ' 1 20 +345 -1278; echo 5
printf '%+-8d ' 1 20 +345 -1278; echo 6
printf '%+08d ' 1 20 +345 -1278; echo 7
printf '%+-08d ' 1 20 +345 -1278; echo 8
printf '%+8.3d ' 1 20 +345 -1278; echo 9
printf '%d ' 00 011 0x1fE 0X1Fe \'a \"@; echo 10
__IN__
0
0
0 10 200 345 -1278 1
+1 +20 +345 -1278 2
+1 +20 +345 -1278 3
 1  20  345 -1278 4
      +1      +20     +345    -1278 5
+1       +20      +345     -1278    6
+0000001 +0000020 +0000345 -0001278 7
+1       +20      +345     -1278    8
    +001     +020     +345    -1278 9
0 9 510 510 97 64 10
__OUT__

test_oE '%i' -e
printf '%i\n'
printf '%i\n' 0
printf '%i ' 0 10 200 +345 -1278; echo 1
printf '%+i ' 1 20 +345 -1278; echo 2
printf '%+ i ' 1 20 +345 -1278; echo 3
printf '% i ' 1 20 +345 -1278; echo 4
printf '%+8i ' 1 20 +345 -1278; echo 5
printf '%+-8i ' 1 20 +345 -1278; echo 6
printf '%+08i ' 1 20 +345 -1278; echo 7
printf '%+-08i ' 1 20 +345 -1278; echo 8
printf '%+8.3i ' 1 20 +345 -1278; echo 9
printf '%i ' 00 011 0x1fE 0X1Fe \'a \"@; echo 10
__IN__
0
0
0 10 200 345 -1278 1
+1 +20 +345 -1278 2
+1 +20 +345 -1278 3
 1  20  345 -1278 4
      +1      +20     +345    -1278 5
+1       +20      +345     -1278    6
+0000001 +0000020 +0000345 -0001278 7
+1       +20      +345     -1278    8
    +001     +020     +345    -1278 9
0 9 510 510 97 64 10
__OUT__

test_oE '%u' -e
printf '%u\n'
printf '%u\n' 0
printf '%u ' 0 10 200 +345; echo 1
printf '%+u ' 1 20 +345; echo 2
printf '%+ u ' 1 20 +345; echo 3
printf '% u ' 1 20 +345; echo 4
printf '%8u ' 1 20 +345; echo 5
printf '%-8u ' 1 20 +345; echo 6
printf '%08u ' 1 20 +345; echo 7
printf '%-08u ' 1 20 +345; echo 8
printf '%8.3u ' 1 20 +345; echo 9
printf '%u ' 00 011 0x1fE 0X1Fe \'a \"@; echo 10
__IN__
0
0
0 10 200 345 1
1 20 345 2
1 20 345 3
1 20 345 4
       1       20      345 5
1        20       345      6
00000001 00000020 00000345 7
1        20       345      8
     001      020      345 9
0 9 510 510 97 64 10
__OUT__

test_oE '%o' -e
printf '%o\n'
printf '%o\n' 0
printf '%o ' 0 10 200 +345; echo 1
printf '%+o ' 1 20 +345; echo 2
printf '%+ o ' 1 20 +345; echo 3
printf '% o ' 1 20 +345; echo 4
printf '%8o ' 1 20 +345; echo 5
printf '%-8o ' 1 20 +345; echo 6
printf '%08o ' 1 20 +345; echo 7
printf '%-08o ' 1 20 +345; echo 8
printf '%8.3o ' 1 20 +345; echo 9
printf '%o ' 00 011 0x1fE 0X1Fe \'a \"@; echo 10
printf '%#.3o ' 0 3 010 0123 012345; echo 11
__IN__
0
0
0 12 310 531 1
1 24 531 2
1 24 531 3
1 24 531 4
       1       24      531 5
1        24       531      6
00000001 00000024 00000531 7
1        24       531      8
     001      024      531 9
0 11 776 776 141 100 10
000 003 010 0123 012345 11
__OUT__

test_oE '%x' -e
printf '%x\n'
printf '%x\n' 0
printf '%x ' 0 10 200 +345; echo 1
printf '%+x ' 1 20 +345; echo 2
printf '%+ x ' 1 20 +345; echo 3
printf '% x ' 1 20 +345; echo 4
printf '%8x ' 1 20 +345; echo 5
printf '%-8x ' 1 20 +345; echo 6
printf '%08x ' 1 20 +345; echo 7
printf '%-08x ' 1 20 +345; echo 8
printf '%8.3x ' 1 20 +345; echo 9
printf '%x ' 00 011 0x1fE 0X1Fe \'a \"@; echo 10
printf '%#x ' 00 011 0x1fE 0X1Fe \'a \"@; echo 11
__IN__
0
0
0 a c8 159 1
1 14 159 2
1 14 159 3
1 14 159 4
       1       14      159 5
1        14       159      6
00000001 00000014 00000159 7
1        14       159      8
     001      014      159 9
0 9 1fe 1fe 61 40 10
0 0x9 0x1fe 0x1fe 0x61 0x40 11
__OUT__

test_oE '%X' -e
printf '%X\n'
printf '%X\n' 0
printf '%X ' 0 10 200 +345; echo 1
printf '%+X ' 1 20 +345; echo 2
printf '%+ X ' 1 20 +345; echo 3
printf '% X ' 1 20 +345; echo 4
printf '%8X ' 1 20 +345; echo 5
printf '%-8X ' 1 20 +345; echo 6
printf '%08X ' 1 20 +345; echo 7
printf '%-08X ' 1 20 +345; echo 8
printf '%8.3X ' 1 20 +345; echo 9
printf '%X ' 00 011 0x1fE 0X1Fe \'a \"@; echo 10
printf '%#X ' 00 011 0x1fE 0X1Fe \'a \"@; echo 11
__IN__
0
0
0 A C8 159 1
1 14 159 2
1 14 159 3
1 14 159 4
       1       14      159 5
1        14       159      6
00000001 00000014 00000159 7
1        14       159      8
     001      014      159 9
0 9 1FE 1FE 61 40 10
0 0X9 0X1FE 0X1FE 0X61 0X40 11
__OUT__

test_oE '%f' -e
printf '%f\n'
printf '%f\n' 0
printf '%f ' 1 1.25 -1000.0; echo 1
printf '%+f ' 1 1.25 -1000.0; echo 2
printf '%+ f ' 1 1.25 -1000.0; echo 3
printf '% f ' 1 1.25 -1000.0; echo 4
printf '%+15f ' 1 1.25 -1000.0; echo 5
printf '%+-15f ' 1 1.25 -1000.0; echo 6
printf '%+015f ' 1 1.25 -1000.0; echo 7
printf '%+-015f ' 1 1.25 -1000.0; echo 8
printf '%+15.3f ' 1 1.25 -1000.0; echo 9
printf '%.0f ' 1 1.25 -1000.0; echo 10
printf '%#.0f ' 1 1.25 -1000.0; echo 11
__IN__
0.000000
0.000000
1.000000 1.250000 -1000.000000 1
+1.000000 +1.250000 -1000.000000 2
+1.000000 +1.250000 -1000.000000 3
 1.000000  1.250000 -1000.000000 4
      +1.000000       +1.250000    -1000.000000 5
+1.000000       +1.250000       -1000.000000    6
+0000001.000000 +0000001.250000 -0001000.000000 7
+1.000000       +1.250000       -1000.000000    8
         +1.000          +1.250       -1000.000 9
1 1 -1000 10
1. 1. -1000. 11
__OUT__

test_oE '%F' -e
printf '%F\n'
printf '%F\n' 0
printf '%F ' 1 1.25 -1000.0; echo 1
printf '%+F ' 1 1.25 -1000.0; echo 2
printf '%+ F ' 1 1.25 -1000.0; echo 3
printf '% F ' 1 1.25 -1000.0; echo 4
printf '%+15F ' 1 1.25 -1000.0; echo 5
printf '%+-15F ' 1 1.25 -1000.0; echo 6
printf '%+015F ' 1 1.25 -1000.0; echo 7
printf '%+-015F ' 1 1.25 -1000.0; echo 8
printf '%+15.3F ' 1 1.25 -1000.0; echo 9
printf '%.0F ' 1 1.25 -1000.0; echo 10
printf '%#.0F ' 1 1.25 -1000.0; echo 11
__IN__
0.000000
0.000000
1.000000 1.250000 -1000.000000 1
+1.000000 +1.250000 -1000.000000 2
+1.000000 +1.250000 -1000.000000 3
 1.000000  1.250000 -1000.000000 4
      +1.000000       +1.250000    -1000.000000 5
+1.000000       +1.250000       -1000.000000    6
+0000001.000000 +0000001.250000 -0001000.000000 7
+1.000000       +1.250000       -1000.000000    8
         +1.000          +1.250       -1000.000 9
1 1 -1000 10
1. 1. -1000. 11
__OUT__

test_oE '%e' -e
printf '%e\n'
printf '%e\n' 0
printf '%e ' 1 1.25 -1000.0; echo 1
printf '%+e ' 1 1.25 -1000.0; echo 2
printf '%+ e ' 1 1.25 -1000.0; echo 3
printf '% e ' 1 1.25 -1000.0; echo 4
printf '%+15e ' 1 1.25 -1000.0; echo 5
printf '%+-15e ' 1 1.25 -1000.0; echo 6
printf '%+015e ' 1 1.25 -1000.0; echo 7
printf '%+-015e ' 1 1.25 -1000.0; echo 8
printf '%+15.3e ' 1 1.25 -1000.0; echo 9
printf '%.0e ' 1 1.25 -1000.0; echo 10
printf '%#.0e ' 1 1.25 -1000.0; echo 11
__IN__
0.000000e+00
0.000000e+00
1.000000e+00 1.250000e+00 -1.000000e+03 1
+1.000000e+00 +1.250000e+00 -1.000000e+03 2
+1.000000e+00 +1.250000e+00 -1.000000e+03 3
 1.000000e+00  1.250000e+00 -1.000000e+03 4
  +1.000000e+00   +1.250000e+00   -1.000000e+03 5
+1.000000e+00   +1.250000e+00   -1.000000e+03   6
+001.000000e+00 +001.250000e+00 -001.000000e+03 7
+1.000000e+00   +1.250000e+00   -1.000000e+03   8
     +1.000e+00      +1.250e+00      -1.000e+03 9
1e+00 1e+00 -1e+03 10
1.e+00 1.e+00 -1.e+03 11
__OUT__

test_oE '%E' -e
printf '%E\n'
printf '%E\n' 0
printf '%E ' 1 1.25 -1000.0; echo 1
printf '%+E ' 1 1.25 -1000.0; echo 2
printf '%+ E ' 1 1.25 -1000.0; echo 3
printf '% E ' 1 1.25 -1000.0; echo 4
printf '%+15E ' 1 1.25 -1000.0; echo 5
printf '%+-15E ' 1 1.25 -1000.0; echo 6
printf '%+015E ' 1 1.25 -1000.0; echo 7
printf '%+-015E ' 1 1.25 -1000.0; echo 8
printf '%+15.3E ' 1 1.25 -1000.0; echo 9
printf '%.0E ' 1 1.25 -1000.0; echo 10
printf '%#.0E ' 1 1.25 -1000.0; echo 11
__IN__
0.000000E+00
0.000000E+00
1.000000E+00 1.250000E+00 -1.000000E+03 1
+1.000000E+00 +1.250000E+00 -1.000000E+03 2
+1.000000E+00 +1.250000E+00 -1.000000E+03 3
 1.000000E+00  1.250000E+00 -1.000000E+03 4
  +1.000000E+00   +1.250000E+00   -1.000000E+03 5
+1.000000E+00   +1.250000E+00   -1.000000E+03   6
+001.000000E+00 +001.250000E+00 -001.000000E+03 7
+1.000000E+00   +1.250000E+00   -1.000000E+03   8
     +1.000E+00      +1.250E+00      -1.000E+03 9
1E+00 1E+00 -1E+03 10
1.E+00 1.E+00 -1.E+03 11
__OUT__

test_oE '%g' -e
printf '%g\n'
printf '%g\n' 0
printf '%g ' 1 1.25 -1000.0 0.000025; echo 1
printf '%+g ' 1 1.25 -1000.0 0.000025; echo 2
printf '%+ g ' 1 1.25 -1000.0 0.000025; echo 3
printf '% g ' 1 1.25 -1000.0 0.000025; echo 4
printf '%+15g ' 1 1.25 -1000.0 0.000025; echo 5
printf '%+-15g ' 1 1.25 -1000.0 0.000025; echo 6
printf '%+015g ' 1 1.25 -1000.0 0.000025; echo 7
printf '%+-015g ' 1 1.25 -1000.0 0.000025; echo 8
printf '%+15.3g ' 1 1.25 -1000.0 0.000025; echo 9
printf '%+#15.3g ' 1 1.25 -1000.0 0.000025; echo 10
__IN__
0
0
1 1.25 -1000 2.5e-05 1
+1 +1.25 -1000 +2.5e-05 2
+1 +1.25 -1000 +2.5e-05 3
 1  1.25 -1000  2.5e-05 4
             +1           +1.25           -1000        +2.5e-05 5
+1              +1.25           -1000           +2.5e-05        6
+00000000000001 +00000000001.25 -00000000001000 +00000002.5e-05 7
+1              +1.25           -1000           +2.5e-05        8
             +1           +1.25          -1e+03        +2.5e-05 9
          +1.00           +1.25       -1.00e+03       +2.50e-05 10
__OUT__

test_oE '%G' -e
printf '%G\n'
printf '%G\n' 0
printf '%G ' 1 1.25 -1000.0 0.000025; echo 1
printf '%+G ' 1 1.25 -1000.0 0.000025; echo 2
printf '%+ G ' 1 1.25 -1000.0 0.000025; echo 3
printf '% G ' 1 1.25 -1000.0 0.000025; echo 4
printf '%+15G ' 1 1.25 -1000.0 0.000025; echo 5
printf '%+-15G ' 1 1.25 -1000.0 0.000025; echo 6
printf '%+015G ' 1 1.25 -1000.0 0.000025; echo 7
printf '%+-015G ' 1 1.25 -1000.0 0.000025; echo 8
printf '%+15.3G ' 1 1.25 -1000.0 0.000025; echo 9
printf '%+#15.3G ' 1 1.25 -1000.0 0.000025; echo 10
__IN__
0
0
1 1.25 -1000 2.5E-05 1
+1 +1.25 -1000 +2.5E-05 2
+1 +1.25 -1000 +2.5E-05 3
 1  1.25 -1000  2.5E-05 4
             +1           +1.25           -1000        +2.5E-05 5
+1              +1.25           -1000           +2.5E-05        6
+00000000000001 +00000000001.25 -00000000001000 +00000002.5E-05 7
+1              +1.25           -1000           +2.5E-05        8
             +1           +1.25          -1E+03        +2.5E-05 9
          +1.00           +1.25       -1.00E+03       +2.50E-05 10
__OUT__

test_oE '%c' -e
printf '%c\n'
printf '%c\n' ''
printf '%c\n' a
printf '%c\n' long arguments
printf '%3c\n' b
printf '%-3c\n' c
__IN__


a
l
a
  b
c  
__OUT__

test_oE '%s' -e
printf '%s\n'
printf '%s\n' 'argument  with  space and
newline'
printf '%s\n' 'argument with backslash \\ and percent %%'
printf '%5s ' 123 a long_argument; echo 1
printf '%5.2s ' 123 a long_argument; echo 2
printf '%-5s ' 123 a long_argument; echo 3
__IN__

argument  with  space and
newline
argument with backslash \\ and percent %%
  123     a long_argument 1
   12     a    lo 2
123   a     long_argument 3
__OUT__

test_oE '%b' -e
printf '%b\n'
printf '%b\n' 'argument  with  space and
newline'
printf '%b\n' 'argument with backslash \\ and percent %%'
printf '%5b ' 123 a long_argument; echo 1
printf '%5.2b ' 123 a long_argument; echo 2
printf '%-5b ' 123 a long_argument; echo 3
printf '%-5.2b ' 123 a long_argument; echo 4
printf '%b\n' '1\a2\b3\c4' 5
printf '%b\n' '6\f7\n8\r9\t0\v!'
printf '%b\n' '\0123\012\01x' '\123\12\1x' '\00411'
__IN__

argument  with  space and
newline
argument with backslash \ and percent %%
  123     a long_argument 1
   12     a    lo 2
123   a     long_argument 3
12    a     lo    4
12367
89	0!
S
x
\123\12\1x
!1
__OUT__

# Many existing shells do not treat this as an error.
test_oE -e 0 'printing value of null character'
printf '%d\n' \' \"
__IN__
0
0
__OUT__

test_oE -e 0 'position' -e
printf '%3$d %4$d %2$d\n' 10 20 30 42
printf '%2$d\n' 1 2 3 4 5
__IN__
30 42 20
2
4
0
__OUT__

# In yash, %b is implemented separately from other conversion specifiers,
# so we test it separately, too.
test_oE -e 0 'position in %b' -e
printf '%3$b %4$b %1$b\n' A BB CCC ddd
printf '%2$b\n' a b c d e
__IN__
CCC ddd A
b
d

__OUT__

test_oE -e 0 'position with flag'
printf '%1$03d\n' 1
__IN__
001
__OUT__

test_oE -e 0 'mixing % and %n$'
printf '%d %5$.3s %5$s %c %d\n' 42 A B C formatted words
__IN__
42 for formatted w 0
__OUT__

test_oE -e 0 'percent'
printf '%%\n'
printf '+%%+%%%%+\n'
__IN__
%
+%+%%+
__OUT__

test_oE -e 0 'percent does not consume operand'
printf '%d%%%d\n' 1 2
__IN__
1%2
__OUT__

test_Oe -e n 'unsupported conversion %y'
printf '%y' 1
__IN__
printf: `y' is not a valid conversion specifier
__ERR__
#'
#`

test_Oe -e n 'missing number before $'
printf '%$d' 1
__IN__
printf: `$' is not a valid conversion specifier
__ERR__
#'
#`

test_Oe -e n 'position 0'
printf '%0$d' 42
__IN__
printf: `$' is not a valid conversion specifier
__ERR__
#'
#`

test_o -d -e n 'operands in invalid format'
printf '%d\n' not_a_integer 32_trailing_characters
__IN__
0
32
__OUT__

test_x -d -e n 'operand overflow is an error'
printf '%d\n' 999999999999999999999999999999999999999999999999999999999999999
__IN__

test_o 'something is printed even in operand overflow'
echo $(
printf '%d\n' 999999999999999999999999999999999999999999999999999999999999999 |
grep -c .
)
__IN__
1
__OUT__

test_oe -e 1 'redundant character in character value notation'
printf '%d\n' 123 \'45 \"67 890
__IN__
123
52
54
890
__OUT__
printf: redundant character in operand `'45'
printf: redundant character in operand `"67'
__ERR__
#'
#"
#`

test_Oe -e 2 'invalid option'
printf --no-such-option ''
__IN__
printf: `--no-such-option' is not a valid option
__ERR__
#'
#`

test_O -d -e 1 'printing to closed stream'
printf '\n' >&-
__IN__

test_Oe -e 2 'missing format'
printf
__IN__
printf: this command requires an operand
__ERR__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
