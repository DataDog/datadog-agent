# fnmatch-p.tst: test of pattern matching for any POSIX-compliant shell

posix="true"

test_oE 'quotations of a normal character'
case a   in a  ) echo 01; esac
case a   in b  ) echo 02; esac
case a   in A  ) echo 03; esac
case \a  in a  ) echo 11; esac
case \a  in b  ) echo 12; esac
case \a  in A  ) echo 13; esac
case a   in \a ) echo 21; esac
case a   in \b ) echo 22; esac
case a   in \A ) echo 23; esac
case \a  in \a ) echo 31; esac
case \a  in \b ) echo 32; esac
case \a  in \A ) echo 33; esac
case 'a' in a  ) echo 41; esac
case 'a' in b  ) echo 42; esac
case 'a' in A  ) echo 43; esac
case a   in 'a') echo 51; esac
case a   in 'b') echo 52; esac
case a   in 'A') echo 53; esac
case 'a' in 'a') echo 61; esac
case 'a' in 'b') echo 62; esac
case 'a' in 'A') echo 63; esac
case "a" in a  ) echo 71; esac
case "a" in b  ) echo 72; esac
case "a" in A  ) echo 73; esac
case a   in "a") echo 81; esac
case a   in "b") echo 82; esac
case a   in "A") echo 83; esac
case "a" in "a") echo 91; esac
case "a" in "b") echo 92; esac
case "a" in "A") echo 93; esac
__IN__
01
11
21
31
41
51
61
71
81
91
__OUT__

test_oE 'quotations of quotations'
sq=\' dq=\" bs=\\
case \'    in \'   ) echo 111; esac
case \'    in "'"  ) echo 112; esac
case \'    in "$sq") echo 113; esac
case "'"   in \'   ) echo 121; esac
case "'"   in "'"  ) echo 122; esac
case "'"   in "$sq") echo 123; esac
case $sq   in \'   ) echo 131; esac
case $sq   in "'"  ) echo 132; esac
case $sq   in "$sq") echo 133; esac
case "$sq" in \'   ) echo 141; esac
case "$sq" in "'"  ) echo 142; esac
case "$sq" in "$sq") echo 143; esac
case \"    in \"   ) echo 211; esac
case \"    in '"'  ) echo 212; esac
case \"    in "$dq") echo 213; esac
case '"'   in \"   ) echo 221; esac
case '"'   in '"'  ) echo 222; esac
case '"'   in "$dq") echo 223; esac
case $dq   in \"   ) echo 231; esac
case $dq   in '"'  ) echo 232; esac
case $dq   in "$dq") echo 233; esac
case "$dq" in \"   ) echo 241; esac
case "$dq" in '"'  ) echo 242; esac
case "$dq" in "$dq") echo 243; esac
case \\    in \\   ) echo 311; esac
case \\    in '\'  ) echo 312; esac
case \\    in "\\" ) echo 313; esac
case \\    in "$bs") echo 314; esac
case '\'   in \\   ) echo 321; esac
case '\'   in '\'  ) echo 322; esac
case '\'   in "\\" ) echo 323; esac
case '\'   in "$bs") echo 324; esac
case "\\"  in \\   ) echo 331; esac
case "\\"  in '\'  ) echo 332; esac
case "\\"  in "\\" ) echo 333; esac
case "\\"  in "$bs") echo 334; esac
case $bs   in \\   ) echo 341; esac
case $bs   in '\'  ) echo 342; esac
case $bs   in "\\" ) echo 343; esac
case $bs   in "$bs") echo 344; esac
case "$bs" in \\   ) echo 351; esac
case "$bs" in '\'  ) echo 352; esac
case "$bs" in "\\" ) echo 353; esac
case "$bs" in "$bs") echo 354; esac
__IN__
111
112
113
121
122
123
131
132
133
141
142
143
211
212
213
221
222
223
231
232
233
241
242
243
311
312
313
314
321
322
323
324
331
332
333
334
341
342
343
344
351
352
353
354
__OUT__

test_oE 'escapes resulting from expansions'
bs=\\ a=*

# $bs* expands to \* which only matches literal *
case x in $bs*) echo not reached 11; esac
case * in $bs*) echo 12; esac

# $bs$a expands to \* which only matches literal *
case x in $bs$a) echo not reached 21; esac
case * in $bs$a) echo 22; esac

# $bs$bs expands to \\ which only matches literal \
case x  in $bs$bs) echo not reached 31; esac
case \\ in $bs$bs) echo 32; esac
__IN__
12
22
32
__OUT__

test_oE 'blanks'
n=
case ''   in ''  ) echo 11; esac
case ''   in ""  ) echo 12; esac
case ''   in $n  ) echo 13; esac
case ''   in "$n") echo 14; esac
case ""   in ''  ) echo 21; esac
case ""   in ""  ) echo 22; esac
case ""   in $n  ) echo 23; esac
case ""   in "$n") echo 24; esac
case $n   in ''  ) echo 31; esac
case $n   in ""  ) echo 32; esac
case $n   in $n  ) echo 33; esac
case $n   in "$n") echo 34; esac
case "$n" in ''  ) echo 41; esac
case "$n" in ""  ) echo 42; esac
case "$n" in $n  ) echo 43; esac
case "$n" in "$n") echo 44; esac
case $n''"""$n" in "$n"""''$n) echo 99; esac
__IN__
11
12
13
14
21
22
23
24
31
32
33
34
41
42
43
44
99
__OUT__

test_oE '? and * and normal characters'
case a  in a ) echo 01; esac
case aa in a ) echo 02; esac
case a  in aa) echo 03; esac
case aa in aa) echo 04; esac
case a  in ? ) echo 11; esac
case a  in * ) echo 12; esac
case a  in ?*) echo 13; esac
case a  in *?) echo 14; esac
case a  in ??) echo 15; esac
case a  in **) echo 16; esac
case aa in ? ) echo 21; esac
case aa in * ) echo 22; esac
case aa in ?*) echo 23; esac
case aa in *?) echo 24; esac
case aa in ??) echo 25; esac
case aa in **) echo 26; esac
__IN__
01
04
11
12
13
14
16
22
23
24
25
26
__OUT__

test_oE '? and * and quotations'
case ''  in ?) echo 01; esac
case ''  in *) echo 02; esac
case \\  in ?) echo 11; esac
case \\  in *) echo 12; esac
case "'" in ?) echo 21; esac
case "'" in *) echo 22; esac
case '"' in ?) echo 31; esac
case '"' in *) echo 32; esac
__IN__
02
11
12
21
22
31
32
__OUT__

test_oE 'brackets'
case a in [[:lower:]])  echo lower ; esac
case a in [[:upper:]])  echo upper ; esac
case a in [[:alpha:]])  echo alpha ; esac
case a in [[:digit:]])  echo digit ; esac
case a in [[:alnum:]])  echo alnum ; esac
case a in [[:punct:]])  echo punct ; esac
case a in [[:graph:]])  echo graph ; esac
case a in [[:print:]])  echo print ; esac
case a in [[:cntrl:]])  echo cntrl ; esac
case a in [[:blank:]])  echo blank ; esac
case a in [[:space:]])  echo space ; esac
case a in [[:xdigit:]]) echo xdigit; esac
case a in [[.a.]]      ) echo 2; esac
case 1 in [0-2]        ) echo 3; esac
case 1 in [[.0.]-[.2.]]) echo 4; esac
case a in [!a]         ) echo 5; esac
case 1 in [!0-2]       ) echo 6; esac
case a in [[=a=]]      ) echo 7; esac
__IN__
lower
alpha
alnum
graph
print
xdigit
2
3
4
7
__OUT__

test_oE 'brackets and quotations'
case \. in ["."]) echo 01; esac
case \[ in ["."]) echo 02; esac
case \" in ["."]) echo 03; esac
case \\ in ["."]) echo 04; esac
case \] in ["."]) echo 05; esac
case \. in [\".]) echo 11; esac
case \[ in [\".]) echo 12; esac
case \" in [\".]) echo 13; esac
case \\ in [\".]) echo 14; esac
case \] in [\".]) echo 15; esac
case \. in [\.] ) echo 21; esac
case \[ in [\.] ) echo 22; esac
case \" in [\.] ) echo 23; esac
case \\ in [\.] ) echo 24; esac
case \] in [\.] ) echo 25; esac
case \. in "[.]") echo 31; esac
case \[ in "[.]") echo 32; esac
case \" in "[.]") echo 33; esac
case \\ in "[.]") echo 34; esac
case \] in "[.]") echo 35; esac
case \. in [\]] ) echo 41; esac
case \[ in [\]] ) echo 42; esac
case \" in [\]] ) echo 43; esac
case \\ in [\]] ) echo 44; esac
case \] in [\]] ) echo 45; esac
case \. in ["]"]) echo 51; esac
case \[ in ["]"]) echo 52; esac
case \" in ["]"]) echo 53; esac
case \\ in ["]"]) echo 54; esac
case \] in ["]"]) echo 55; esac
__IN__
01
11
13
21
45
55
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
