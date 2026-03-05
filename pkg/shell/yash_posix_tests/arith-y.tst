# arith-y.tst: yash-specific test of arithmetic expansion

setup -d

# POSIX says, "The expression shall be treated as if it were in double-quotes,
# except that a double-quote inside the expression is not treated specially."
# This means single- and double-quotes are not special and backslashes are
# special only before a dollar, backquote, backslash, or newline.
# Since no combinations of such a backslash and escaped character produce a
# valid arithmetic expression, we only test for errors here.
# Note that line continuation is tested in quote-p.tst.

test_Oe -e 2 'quote removal: \$'
eval 'echoraw $((\$a))'
__IN__
eval: arithmetic: `$' is not a valid number or operator
eval: arithmetic: a value is missing
__ERR__
#'
#`

test_Oe -e 2 'quote removal: \`'
eval 'echoraw $((\`echo\`))'
__IN__
eval: arithmetic: ``' is not a valid number or operator
eval: arithmetic: a value is missing
__ERR__
#'
#`

test_Oe -e 2 'quote removal: \\'
eval 'echoraw $((\\))'
__IN__
eval: arithmetic: `\' is not a valid number or operator
eval: arithmetic: a value is missing
__ERR__
#'
#`

test_Oe -e 2 'quote removal: \" is not special'
eval 'echoraw $((\"3\"))'
__IN__
eval: arithmetic: `\' is not a valid number or operator
eval: arithmetic: a value is missing
__ERR__
#'
#`

test_Oe -e 2 'quote removal: \a is not special'
eval 'echoraw $((\a))'
__IN__
eval: arithmetic: `\' is not a valid number or operator
eval: arithmetic: a value is missing
__ERR__
#'
#`

test_oE -e 0 'single variable'
unset unset
empty= zero=0.0 one=1.0 nonnumeric='hello  world'
echoraw "[$((empty))]" $((zero)) $((one)) "$((nonnumeric))" $((unset))
__IN__
[] 0.0 1.0 hello  world 0
__OUT__

test_oE 'integer: prefix ++'
x=1
echo $((++x))
echo $((++x))
echo $((++x))
echo $x
__IN__
2
3
4
4
__OUT__

test_oE 'integer: prefix --'
x=1
echo $((--x))
echo $((--x))
echo $((--x))
echo $x
__IN__
0
-1
-2
-2
__OUT__

test_oE 'integer: postfix ++'
x=1
echo $((x++))
echo $((x++))
echo $((x++))
echo $x
__IN__
1
2
3
4
__OUT__

test_oE 'integer: postfix --'
x=1
echo $((x--))
echo $((x--))
echo $((x--))
echo $x
__IN__
1
0
-1
-2
__OUT__

# $1 = line no.
# $2 = arithmetic expression that causes division by zero
test_division_by_zero() {
    testcase "$1" -e 2 "division by zero ($2)" 3<<__IN__ 4</dev/null 5<<\__ERR__
eval 'echo \$(($2))'
__IN__
eval: arithmetic: division by zero
__ERR__
}

test_division_by_zero "$LINENO" '0/0'
test_division_by_zero "$LINENO" '0%0'
test_division_by_zero "$LINENO" '1/0'
test_division_by_zero "$LINENO" '1%0'
(
setup 'x=0'
test_division_by_zero "$LINENO" 'x/=0'
test_division_by_zero "$LINENO" 'x%=0'
)
(
setup 'x=1'
test_division_by_zero "$LINENO" 'x/=0'
test_division_by_zero "$LINENO" 'x%=0'
)

test_oE -e 0 'float: single constant'
echoraw $((0.0)) $((1.0)) $((100.)) $((020.)) $((.250)) $((7.5e-1))
__IN__
0 1 100 20 0.25 0.75
__OUT__

test_oE -e 0 'float: unary plus operator'
echoraw $((+1.5))
__IN__
1.5
__OUT__

test_oE -e 0 'float: unary minus operator'
echoraw $((-1.5))
__IN__
-1.5
__OUT__

test_oE -e 0 'float: unary binary negation operator'
echoraw $((~5.75))
__IN__
-6
__OUT__

test_oE -e 0 'float: unary logical negation operator'
echoraw $((!5.75)) $((!0.0)) $((!0.25)) $((!1.0))
__IN__
0 1 0 0
__OUT__

test_oE -e 0 'float: multiplicative operators'
echoraw $((3.5*4.5)) $((3*4.5)) $((3.5*4))
echoraw $((15.75/3.5)) $((13.5/3)) $((14/4.0))
echoraw $((6.5%2.5)) $((11.5%5)) $((12%3.5))
__IN__
15.75 13.5 14
4.5 4.5 3.5
1.5 1.5 1.5
__OUT__

test_oE -e 0 'float: additive operators'
echoraw $((1.5+2.25)) $((1.5+2)) $((1+2.25))
echoraw $((1.5-2.25)) $((1.5-2)) $((1-2.25))
__IN__
3.75 3.5 3.25
-0.75 -0.5 -1.25
__OUT__

test_oE -e 0 'float: shift operators'
echoraw $((5.5<<1.75)) $((5.5<<1)) $((5<<1.75))
echoraw $((3.0>>1.0)) $((3.0>>1)) $((3>>1.0))
__IN__
10 10 10
1 1 1
__OUT__

test_oE -e 0 'float: relational operators: 1'
echoraw $((0.0 < 0.0)) $((0.0 <= 0.0)) $((0.0 > 0.0)) $((0.0 >= 0.0))
echoraw $((0   < 0.0)) $((0   <= 0.0)) $((0   > 0.0)) $((0   >= 0.0))
echoraw $((0.0 < 0  )) $((0.0 <= 0  )) $((0.0 > 0  )) $((0.0 >= 0  ))
__IN__
0 1 0 1
0 1 0 1
0 1 0 1
__OUT__

test_oE -e 0 'float: relational operators: 2'
echoraw $((.25 < .75)) $((.25 <= .75)) $((.25 > .75)) $((.25 >= .75))
echoraw $((0   < .75)) $((0   <= .75)) $((0   > .75)) $((0   >= .75))
echoraw $((.25 < 1  )) $((.25 <= 1  )) $((.25 > 1  )) $((.25 >= 1  ))
__IN__
1 1 0 0
1 1 0 0
1 1 0 0
__OUT__

test_oE -e 0 'float: relational operators: 3'
echoraw $((.75 < .25)) $((.75 <= .25)) $((.75 > .25)) $((.75 >= .25))
echoraw $((1   < .25)) $((1   <= .25)) $((1   > .25)) $((1   >= .25))
echoraw $((.75 < 0  )) $((.75 <= 0  )) $((.75 > 0  )) $((.75 >= 0  ))
__IN__
0 0 1 1
0 0 1 1
0 0 1 1
__OUT__

test_oE -e 0 'float: equality operators'
echoraw $((0.0==0.0)) $((0.0==0.1)) $((0.1==0.0)) $((0.1==0.1))
echoraw $((0  ==0.0)) $((0  ==0.1)) $((1  ==0.0)) $((1  ==0.1))
echoraw $((0.0==0  )) $((0.0==1  )) $((0.1==0  )) $((0.1==1  ))
echoraw $((0.0!=0.0)) $((0.0!=0.1)) $((0.1!=0.0)) $((0.1!=0.1))
echoraw $((0  !=0.0)) $((0  !=0.1)) $((1  !=0.0)) $((1  !=0.1))
echoraw $((0.0!=0  )) $((0.0!=1  )) $((0.1!=0  )) $((0.1!=1  ))
__IN__
1 0 0 1
1 0 0 0
1 0 0 0
0 1 1 0
0 1 1 1
0 1 1 1
__OUT__

test_oE -e 0 'float: bitwise operators'
echoraw $((10.75 & 12.25)) $((10.75 & 12)) $((10 & 12.25))
echoraw $((10.75 ^ 12.25)) $((10.75 ^ 12)) $((10 ^ 12.25))
echoraw $((10.75 | 12.25)) $((10.75 | 12)) $((10 | 12.25))
__IN__
8 8 8
6 6 6
14 14 14
__OUT__

test_oE -e 0 'float: logical operators'
echoraw $((0.1 && 0.5)) $((0.0 && 0.5)) $((0.1 && 0.0)) $((0.0 && 0.0))
echoraw $((0.1 || 0.5)) $((0.0 || 0.5)) $((0.1 || 0.0)) $((0.0 || 0.0))
a=A b=B
echoraw $((0.1 ? a : b)) $((0.0 ? a : b))
__IN__
1 0 0 0
1 1 1 0
A B
__OUT__

test_oE -e 0 'float: assignment operator ='
a=1 b=1.2 c=1.2 d=foo
echoraw $((a=2.5)) $((b=3.75)) $((c=9)) $((d=1.25))
echoraw $a $b $c $d
__IN__
2.5 3.75 9 1.25
2.5 3.75 9 1.25
__OUT__

test_oE -e 0 'float: assignment operator *='
a=1 b=1.5 c=1.5
echoraw $((a*=2.5)) $((b*=3.75)) $((c*=9))
echoraw $a $b $c
__IN__
2.5 5.625 13.5
2.5 5.625 13.5
__OUT__

test_oE -e 0 'float: assignment operator /='
a=50 b=3.75 c=2.5
echoraw $((a/=1.25)) $((b/=1.5)) $((c/=2))
echoraw $a $b $c
__IN__
40 2.5 1.25
40 2.5 1.25
__OUT__

test_oE -e 0 'float: assignment operator %='
a=7 b=7.25 c=5.5
echoraw $((a%=1.25)) $((b%=1.25)) $((c%=2))
echoraw $a $b $c
__IN__
0.75 1 1.5
0.75 1 1.5
__OUT__

test_oE -e 0 'float: assignment operator +='
a=7 b=7.25 c=5.5
echoraw $((a+=1.25)) $((b+=1.25)) $((c+=2))
echoraw $a $b $c
__IN__
8.25 8.5 7.5
8.25 8.5 7.5
__OUT__

test_oE -e 0 'float: assignment operator -='
a=7 b=7.25 c=5.5
echoraw $((a-=1.25)) $((b-=1.5)) $((c-=2))
echoraw $a $b $c
__IN__
5.75 5.75 3.5
5.75 5.75 3.5
__OUT__

test_oE -e 0 'float: assignment operator <<='
a=10 b=10.9 c=10.9
echoraw $((a<<=2.9)) $((b<<=2.9)) $((c<<=2))
echoraw $a $b $c
__IN__
40 40 40
40 40 40
__OUT__

test_oE -e 0 'float: assignment operator >>='
a=42 b=42.9 c=42.9
echoraw $((a>>=2.9)) $((b>>=2.9)) $((c>>=2))
echoraw $a $b $c
__IN__
10 10 10
10 10 10
__OUT__

test_oE -e 0 'float: assignment operator &='
a=10 b=10.75 c=10.75
echoraw $((a&=12.25)) $((b&=12.25)) $((c&=12))
echoraw $a $b $c
__IN__
8 8 8
8 8 8
__OUT__

test_oE -e 0 'float: assignment operator ^='
a=10 b=10.75 c=10.75
echoraw $((a^=12.25)) $((b^=12.25)) $((c^=12))
echoraw $a $b $c
__IN__
6 6 6
6 6 6
__OUT__

test_oE -e 0 'float: assignment operator |='
a=10 b=10.75 c=10.75
echoraw $((a|=12.25)) $((b|=12.25)) $((c|=12))
echoraw $a $b $c
__IN__
14 14 14
14 14 14
__OUT__

test_Oe -e 2 'empty arithmetic expansion'
eval '$(())'
__IN__
eval: arithmetic: a value is missing
__ERR__

(
posix=true

test_Oe -e 2 'float literal in POSIXly-correct mode'
eval 'echoraw $((1.5))'
__IN__
eval: arithmetic: `1.5' is not a valid number
__ERR__
#'
#`

test_Oe -e 2 'float variable in POSIXly-correct mode'
a=1.5
eval 'echoraw $((-a))'
__IN__
eval: arithmetic: `1.5' is not a valid number
__ERR__
#'
#`

test_Oe -e 2 'prefix ++ in POSIXly-correct mode'
eval 'echoraw $((++a))'
__IN__
eval: arithmetic: operator `++' is not supported
__ERR__
#'
#`

test_Oe -e 2 'prefix -- in POSIXly-correct mode'
eval 'echoraw $((--a))'
__IN__
eval: arithmetic: operator `--' is not supported
__ERR__
#'
#`

test_Oe -e 2 'postfix ++ in POSIXly-correct mode'
eval 'echoraw $((a++))'
__IN__
eval: arithmetic: operator `++' is not supported
__ERR__
#'
#`

test_Oe -e 2 'postfix -- in POSIXly-correct mode'
eval 'echoraw $((a--))'
__IN__
eval: arithmetic: operator `--' is not supported
__ERR__
#'
#`

)

# vim: set ft=sh ts=8 sts=4 sw=4 et:
