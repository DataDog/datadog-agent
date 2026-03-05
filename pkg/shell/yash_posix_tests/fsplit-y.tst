# fsplit-p.tst: yash-specific test of field splitting

setup -d

test_o 'modified IFS affects following expansions in single simple command'
unset IFS
v='1  2  3'
bracket $v "${IFS=X}" $v
__IN__
[1][2][3][X][1  2  3]
__OUT__

test_oE 'empty last field is not ignored (non-backslash IFS)' --empty-last-field
IFS=' ='
a='='; bracket $a
a='=='; bracket $a
a='==='; bracket $a
a='1'; bracket $a
a='1='; bracket $a
a='1=='; bracket $a
a='1==='; bracket $a
echo ===
a='1= '; bracket $a
a='1==  '; bracket $a
a='1===   '; bracket $a
echo ===
a='1= ='; bracket $a
a='1==  ='; bracket $a
a='1===   ='; bracket $a
__IN__
[][]
[][][]
[][][][]
[1]
[1][]
[1][][]
[1][][][]
===
[1][]
[1][][]
[1][][][]
===
[1][][]
[1][][][]
[1][][][][]
__OUT__

test_oE 'empty last field is not ignored (backslash IFS)' --empty-last-field
IFS=' =\'
a='\'; bracket $a
a='\\'; bracket $a
a='\\\'; bracket $a
a='1'; bracket $a
a='1\'; bracket $a
a='1\\'; bracket $a
a='1\\\'; bracket $a
echo ===
a='1\ '; bracket $a
a='1\\  '; bracket $a
a='1\\\   '; bracket $a
echo ===
a='1\ \'; bracket $a
a='1\\  \'; bracket $a
a='1\\\   \'; bracket $a
__IN__
[][]
[][][]
[][][][]
[1]
[1][]
[1][][]
[1][][][]
===
[1][]
[1][][]
[1][][][]
===
[1][][]
[1][][][]
[1][][][][]
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
