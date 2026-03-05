# arith-p.tst: test of arithmetic expansion for any POSIX-compliant shell

posix="true"
setup -d

# POSIX does not specify how the result of an arithmetic expansion should be
# formatted. We assume the result is always formatted by 'printf "%ld"'.

test_oE -e 0 'single constant'
echoraw $((0)) $((1)) $((100)) $((020)) $((0x7F))
__IN__
0 1 100 16 127
__OUT__

test_oE -e 0 'single variable'
zero=0 one=1 hundred=100 plus_one=+1 minus_one=-1
echoraw $((zero)) $((one)) $((hundred)) $((plus_one)) $((minus_one))
__IN__
0 1 100 1 -1
__OUT__

test_oE -e 0 'unset variable is considered 0 (direct)'
unset x
echoraw $((x))
__IN__
0
__OUT__

test_oE -e 0 'unary sign operators'
echoraw $((+1)) $((-1)) $((-+-2))
__IN__
1 -1 2
__OUT__

test_oE -e 0 'unary negation operators'
echoraw $((~0)) $((~1)) $((~2)) $((~-1)) $((~-2))
echoraw $((!0)) $((!1)) $((!2)) $((!-1)) $((!-2))
__IN__
-1 -2 -3 0 1
1 0 0 0 0
__OUT__

test_oE -e 0 'multiplicative operators'
echoraw $((0 * 0)) $((1*0)) $((0*1)) $((1*1)) $((2*3)) $((-5*7))
echoraw $((0 / 1)) $((6/2)) $((-12/3)) $((35/-5)) $((-121/-11))
echoraw $((0 % 1)) $((1%1)) $((1%2)) $((47%7))
__IN__
0 0 0 1 6 -35
0 3 -4 -7 11
0 0 1 5
__OUT__

test_oE -e 0 'additive operators'
echoraw $((0 + 0)) $((0+1)) $((2+3)) $((5+-7)) $((-7+13)) $((-1+-2))
echoraw $((0 - 0)) $((0-1)) $((3-2)) $((5- -7)) $((-7-13)) $((-1- -2))
__IN__
0 1 5 -2 6 -3
0 -1 1 12 -20 1
__OUT__

test_oE -e 0 'shift operators'
echoraw $((0 << 0)) $((3<<2)) $((5<<3)) # undefined: $((-2<<3))
echoraw $((0 >> 0)) $((15>>2)) $((43>>3)) $((-14>>3))
__IN__
0 12 40
0 3 5 -2
__OUT__

test_oE -e 0 'relational operators'
echoraw $((0 < 0)) $((0 <= 0)) $((0 > 0)) $((0 >= 0))
echoraw $((-1< -1)) $((-1< 0)) $((-1< 1)) \
        $(( 0< -1)) $(( 0< 0)) $(( 0< 1)) \
        $(( 1< -1)) $(( 1< 0)) $(( 1< 1))
echoraw $((-1<=-1)) $((-1<=0)) $((-1<=1)) \
        $(( 0<=-1)) $(( 0<=0)) $(( 0<=1)) \
        $(( 1<=-1)) $(( 1<=0)) $(( 1<=1))
echoraw $((-1> -1)) $((-1> 0)) $((-1> 1)) \
        $(( 0> -1)) $(( 0> 0)) $(( 0> 1)) \
        $(( 1> -1)) $(( 1> 0)) $(( 1> 1))
echoraw $((-1>=-1)) $((-1>=0)) $((-1>=1)) \
        $(( 0>=-1)) $(( 0>=0)) $(( 0>=1)) \
        $(( 1>=-1)) $(( 1>=0)) $(( 1>=1))
__IN__
0 1 0 1
0 1 1 0 0 1 0 0 0
1 1 1 0 1 1 0 0 1
0 0 0 1 0 0 1 1 0
1 0 0 1 1 0 1 1 1
__OUT__

test_oE -e 0 'equality operators'
echoraw $((0 == 0)) $((1==0)) $((0==1)) $((1==1)) $((3==3)) $((2==3))
echoraw $((0 != 0)) $((1!=0)) $((0!=1)) $((1!=1)) $((3!=3)) $((2!=3))
__IN__
1 0 0 1 1 0
0 1 1 0 0 1
__OUT__

test_oE -e 0 'bitwise operators'
echoraw $((0 & 0)) $((3&5)) $((-13&5)) $((3&-11)) $((-13&-11))
echoraw $((0 ^ 0)) $((3^5)) $((-13^5)) $((3^-11)) $((-13^-11))
echoraw $((0 | 0)) $((3|5)) $((-13|5)) $((3|-11)) $((-13|-11))
__IN__
0 1 1 1 -15
0 6 -10 -10 6
0 7 -9 -9 -9
__OUT__

test_oE -e 0 'logical operators'
echoraw $((0 && 0)) $((3&&0)) $((0&&-5)) $((3&&-5))
echoraw $((0 || 0)) $((3||0)) $((0||-5)) $((3||-5))
echoraw $((0 ? 0 : 0)) $((0?1:2)) $((1?2:3)) $((-1?2:3))
__IN__
0 0 0 1
0 1 1 1
0 2 2 2
__OUT__

test_oE -e 0 'conditional evaluation of && operator operand'
a=0
echoraw $((1&&(a=5)))
echoraw $((0&&(a=-5)))
echoraw $a
__IN__
1
0
5
__OUT__

test_oE -e 0 'conditional evaluation of || operator operand'
a=0
echoraw $((0||(a=5)))
echoraw $((1||(a=-5)))
echoraw $a
__IN__
1
1
5
__OUT__

test_oE -e 0 'conditional evaluation of ?: operator operand'
a=0 b=0
echoraw $((1?(a=5):(b=-5)))
echoraw $a $b
a=0 b=0
echoraw $((0?(a=-5):(b=5)))
echoraw $a $b
__IN__
5
5 0
5
0 5
__OUT__

test_oE -e 0 'assignment operators'
a=0 b=2 c=15 d=46 e=3 f=3 g=7 h=30 i=3 j=3 k=3
echoraw $((a=5)) $((b*=3)) $((c/=3)) $((d%=7)) $((e+=5)) $((f-=5)) \
    $((g<<=2)) $((h>>=2)) $((i&=5)) $((j^=5)) $((k|=5))
echoraw $a $b $c $d $e $f $g $h $i $j $k
__IN__
5 6 5 4 8 -2 28 7 1 6 7
5 6 5 4 8 -2 28 7 1 6 7
__OUT__

test_O -d -e n 'assigning to read-only variable'
readonly a=3
echoraw $((a=5))
echoraw not reached
__IN__

test_oE -e 0 'unset variable is considered 0 (assignment)'
unset x
echoraw $((a=x)) && echoraw $a
__IN__
0
0
__OUT__

test_oE 'operator precedence: unary and multiplicatives'
echoraw $((!0*3)) $((~-1*3)) $((!1/2)) $((!1%1))
__IN__
3 0 0 0
__OUT__

test_oE 'operator precedence: multiplicatives'
echoraw -          -           $((2*1%2))
echoraw $((2/2*3)) $((12/6/2)) $((6/3%2))
echoraw $((7%4*2)) $((8%12/2)) $((7%2%3))
__IN__
- - 0
3 1 0
6 4 1
__OUT__

test_oE 'operator precedence: multiplicatives and additives'
echoraw $((2*3+1)) $((2*3-1)) $((1/1+1)) $((5%1+1))
echoraw $((1+2*3)) $((9-2*3)) $((2+0/2)) $((1+5%1))
__IN__
7 5 2 1
7 3 2 1
__OUT__

test_oE 'operator precedence: additives'
echoraw $((2-1+3)) $((3-2-1))
__IN__
4 0
__OUT__

test_oE 'operator precedence: additives and shifts'
echoraw $((1+1<<2)) $((3-1<<2)) $((8+8>>2)) $((8-4>>2))
echoraw $((1<<1+1)) $((2<<1-1)) $((8>>1+1)) $((8>>1-1))
__IN__
8 8 4 1
4 2 2 8
__OUT__

test_oE 'operator precedence: shifts'
echoraw $((1<<2<<1)) $((1<<3>>1)) $((8>>2<<1)) $((8>>2>>1))
__IN__
8 4 4 1
__OUT__

test_oE 'operator precedence: shifts and relationals'
echoraw $((1<<1<0)) $((2>>1<=0)) $((1<<1>2)) $((2>>1>=2))
echoraw $((0<2>>1)) $((1<=1<<1)) $((1>1>>1)) $((1>=1<<1))
__IN__
0 0 0 0
1 1 1 0
__OUT__

test_oE 'operator precedence: relationals'
echoraw $((1< 2< 2)) $((0< 1<=2)) $((1< 2> 0)) $((1< 2>=0))
echoraw $((1<=2< 2)) $((0<=0<=0)) $((1<=2> 1)) $((1<=2>=2))
echoraw $((0> 0< 1)) $((0> 0<=1)) $((1> 2> 2)) $((1> 2>=0))
echoraw $((0>=0< 0)) $((0>=0<=1)) $((1>=2> 1)) $((1>=2>=2))
__IN__
1 1 1 1
1 0 0 0
1 1 0 1
0 1 0 0
__OUT__

test_oE 'operator precedence: relationals and equalities'
echoraw $((0<=0==0)) $((0<=0!=1)) $((0==0<=0)) $((0!=2<=1))
echoraw $((1< 0==0)) $((1< 0!=1)) $((0==0< 0)) $((1!=0< 0))
echoraw $((0>=0==0)) $((1>=2!=0)) $((0==0>=0)) $((1!=0>=0))
echoraw $((0> 0==0)) $((0> 0!=1)) $((0==0> 1)) $((1!=0> 1))
__IN__
0 0 0 0
1 1 1 1
0 0 0 0
1 1 1 1
__OUT__

test_oE 'operator precedence: equalities'
echoraw $((0==0==2)) $((0==0!=2)) $((2!=0==0)) $((0!=2!=2))
__IN__
0 1 0 1
__OUT__

test_oE 'operator precedence: equalities and bitwise and'
echoraw $((0==0&0)) $((0&0==0)) $((1!=0&0)) $((0&0!=1))
__IN__
0 0 0 0
__OUT__

test_oE 'operator precedence: bitwise and and xor'
echoraw $((0&0^1)) $((1^0&0))
__IN__
1 1
__OUT__

test_oE 'operator precedence: bitwise xor and or'
echoraw $((1^1|1)) $((1|1^1))
__IN__
1 1
__OUT__

test_oE 'operator precedence: bitwise or and logical and'
echoraw $((1|1&&0)) $((0&&1|1))
__IN__
0 0
__OUT__

test_oE 'operator precedence: logical and and or'
echoraw $((0&&0||1)) $((1||0&&0))
__IN__
1 1
__OUT__

test_oE 'operator precedence: logical or and conditional'
echoraw $((2||0?0:1)) $((1?0:1||1))
__IN__
0 0
__OUT__

test_oE 'operator precedence: conditionals'
echoraw $((4?1:0?2:3))
__IN__
1
__OUT__

test_oE 'operator precedence: assignments in conditionals'
b=5 c=6 d=5 e=0 f=0 g=1 h=8 i=7 j=0 k=0
echoraw $((1?a=2:3)) $((1?b*=2:3)) $((1?c/=2:3)) $((1?d%=2:3)) \
    $((1?e+=2:3)) $((1?f-=2:3)) $((1?g<<=2:3)) $((1?h>>=2:3)) \
    $((1?i&=2:4)) $((1?j^=2:4)) $((1?k|=2:4))
echoraw $a $b $c $d $e $f $g $h $i $j $k
__IN__
2 10 3 1 2 -2 4 2 2 2 2
2 10 3 1 2 -2 4 2 2 2 2
__OUT__

test_oE 'operator precedence: conditionals and assignments'
b=5 c=6 d=5 e=0 f=0 g=1 h=8 i=7 j=0 k=0
echoraw $((a=1?2:3)) $((b*=1?2:3)) $((c/=1?2:3)) $((d%=1?2:3)) \
    $((e+=1?2:3)) $((f-=1?2:3)) $((g<<=1?2:3)) $((h>>=1?2:3)) \
    $((i&=1?2:4)) $((j^=1?2:4)) $((k|=1?2:4))
echoraw $a $b $c $d $e $f $g $h $i $j $k
__IN__
2 10 3 1 2 -2 4 2 2 2 2
2 10 3 1 2 -2 4 2 2 2 2
__OUT__

test_oE 'operator precedence: assignments'
b=5 c=10 d=5 e=10 f=10 g=16 h=2 i=3 j=1 k=2
echoraw $((a=b*=2)) $((c/=d%=3)) $((e+=f-=1)) $((g<<=h>>=1)) $((i&=j^=k|=1))
echoraw $a $b $c $d $e $f $g $h $i $j $k
__IN__
10 5 19 32 2
10 10 5 2 19 9 32 1 2 2 3
__OUT__

test_oE 'parentheses'
echoraw $(((7))) $((-(-3))) $(((1+2)*5)) $((15/(7%4))) $(((0?1:0)?1:(a=2)))
__IN__
7 3 15 5 2
__OUT__

test_oE 'parameter expansion in arithmetic expansion'
a=+123
echoraw $(($a)) $((${a%3})) $(($a-23))
__IN__
123 12 100
__OUT__

test_oE 'command substitution in arithmetic expansion'
echoraw $(($(echo 123))) $((1+$(echo 10)+`echo 100`+1000))
__IN__
123 1111
__OUT__

# Quote removal is tested in arith-y.tst.
#test_oE 'quote removal'
#__IN__
#__OUT__

test_oE 'assignment in parameter expansion in arithmetic expansion'
unset a
echoraw $((${a=1}))
echoraw $a
__IN__
1
1
__OUT__

test_O -d -e n 'malformed arithmetic expansion'
echoraw $((--))
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
