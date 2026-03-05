# declutil-y.tst: yash-specific test of declaration utilities

>tmpfile

# local is a declaration utility in yash
test_oE 'no pathname expansion or field splitting in local A=$a'
a="1  *  2"
local A=$a
printf "%s\n" "$A"
__IN__
1  *  2
__OUT__

# typeset is a declaration utility in yash
test_oE 'no pathname expansion or field splitting in typeset A=$a'
a="1  *  2"
typeset A=$a
printf "%s\n" "$A"
__IN__
1  *  2
__OUT__

# printf is not a declaration utility in yash
test_oE 'pathname expansion and field splitting in printf A=$a'
a='1  tmp*  2'
printf "%s\n" A=$a
__IN__
A=1
tmpfile
2
__OUT__

# printf is not a declaration utility in yash
test_oE 'tilde expansions in printf A=~:~'
HOME=/foo
printf "%s\n" A=~:~
__IN__
A=~:~
__OUT__

# printf is not a declaration utility in yash
test_oE 'command command printf'
a='1  tmp*  2'
command command printf "%s\n" A=$a
__IN__
A=1
tmpfile
2
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
