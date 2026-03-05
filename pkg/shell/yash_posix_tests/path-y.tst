# path-y.tst: yash-specific test of pathname expansion

>Caseglob1 >caseglob2

test_oE 'caseglob on: effect' --caseglob
echo caseglob*
__IN__
caseglob2
__OUT__

test_oE 'caseglob off: effect' --nocaseglob
echo caseglob*
echo Caseglob*
__IN__
Caseglob1 caseglob2
Caseglob1 caseglob2
__OUT__

(
mkdir dotglob
cd dotglob
>.dotglob
)

(
setup 'cd dotglob'

test_oE 'dotglob on: effect' --dotglob
echo *
echo ?dotglob
__IN__
.dotglob
.dotglob
__OUT__

test_oE 'dotglob off: effect' --nodotglob
echo *
echo ?dotglob
__IN__
*
?dotglob
__OUT__

)

(
mkdir markdirs
cd markdirs
>regular
mkdir directory
)

(
setup 'cd markdirs'

test_oE 'markdirs on: effect' --markdirs
echo *r*
__IN__
directory/ regular
__OUT__

test_oE 'markdirs off: effect' --nomarkdirs
echo *r*
__IN__
directory regular
__OUT__

)

(
mkdir extendedglob
cd extendedglob
mkdir dir dir/dir dir/.dir anotherdir .dir .dir/dir
>dir/dir/file >dir/.dir/file >anotherdir/file >.dir/file >.dir/dir/file
ln -s ../../anotherdir dir/dir/link
ln -s ../../anotherdir dir/dir/.link
ln -s ../dir anotherdir/loop
)

(
setup 'cd extendedglob'

test_oE 'extendedglob on: effect' --extendedglob
echo **/file
echo ***/file
echo .**/file
echo .***/file
echo **/**/f*e
__IN__
anotherdir/file dir/dir/file
anotherdir/file anotherdir/loop/dir/file dir/dir/file dir/dir/link/file
.dir/dir/file .dir/file anotherdir/file dir/.dir/file dir/dir/file
.dir/dir/file .dir/file anotherdir/file anotherdir/loop/.dir/file anotherdir/loop/dir/file dir/.dir/file dir/dir/.link/file dir/dir/file dir/dir/link/file
anotherdir/file dir/dir/file
__OUT__

test_oE 'extendedglob off: effect' --noextendedglob
echo **/file
echo ***/file
echo .**/file
echo .***/file
echo **/**/f*e
__IN__
anotherdir/file
anotherdir/file
.dir/file
.dir/file
dir/dir/file
__OUT__

test_oE 'no pattern matches . or ..'
echo .*/ # should not print . or ..
__IN__
.dir/
__OUT__
)

(
mkdir extendedglob2
cd extendedglob2
mkdir -p a/a/a a/a/b a/b/a a/b/b b/a/a b/a/b b/b/a b/b/b
for d in */*/*; do (cd -- "$d"; ln -s ../../.. a; ln -s ../../.. b) done
)

(
setup 'cd extendedglob2'

test_oE 'complicated extendedglob a/b/**/a' --extendedglob
printf '%s\n' a/b/**/a
__IN__
a/b/a
a/b/a/a
a/b/b/a
__OUT__

test_oE 'complicated extendedglob a/b/***/a' --extendedglob
printf '%s\n' a/b/***/a
__IN__
a/b/a
a/b/a/a
a/b/a/a/a
a/b/a/a/a/a
a/b/a/a/a/a/a
a/b/a/a/a/a/a/a
a/b/a/a/a/a/b/a
a/b/a/a/a/b/a
a/b/a/a/a/b/b/a
a/b/a/a/b/a
a/b/a/a/b/a/a
a/b/a/a/b/a/a/a
a/b/a/a/b/a/b/a
a/b/a/a/b/b/a
a/b/a/a/b/b/a/a
a/b/a/a/b/b/b/a
a/b/a/b/a
a/b/a/b/a/a
a/b/a/b/a/a/a
a/b/a/b/a/a/a/a
a/b/a/b/a/a/b/a
a/b/a/b/a/b/a
a/b/a/b/a/b/b/a
a/b/a/b/b/a
a/b/a/b/b/a/a
a/b/a/b/b/a/a/a
a/b/a/b/b/a/b/a
a/b/a/b/b/b/a
a/b/a/b/b/b/a/a
a/b/a/b/b/b/b/a
a/b/b/a
a/b/b/a/a
a/b/b/a/a/a
a/b/b/a/a/a/a
a/b/b/a/a/a/a/a
a/b/b/a/a/a/b/a
a/b/b/a/a/b/a
a/b/b/a/a/b/a/a
a/b/b/a/b/a
a/b/b/a/b/a/a
a/b/b/a/b/a/a/a
a/b/b/a/b/a/b/a
a/b/b/a/b/b/a
a/b/b/a/b/b/a/a
a/b/b/a/b/b/b/a
a/b/b/b/a
a/b/b/b/a/a
a/b/b/b/a/a/a
a/b/b/b/a/a/a/a
a/b/b/b/a/a/b/a
a/b/b/b/a/b/a
a/b/b/b/a/b/a/a
a/b/b/b/b/a
a/b/b/b/b/a/a
a/b/b/b/b/a/a/a
a/b/b/b/b/a/b/a
a/b/b/b/b/b/a
a/b/b/b/b/b/a/a
a/b/b/b/b/b/b/a
__OUT__

test_oE 'complicated extendedglob **/a/a/b' --extendedglob
printf '%s\n' **/a/a/b
__IN__
a/a/a/a/a/b
a/a/a/a/b
a/a/a/b
a/a/b
a/a/b/a/a/b
a/b/a/a/a/b
a/b/a/a/b
a/b/b/a/a/b
b/a/a/a/a/b
b/a/a/a/b
b/a/a/b
b/a/b/a/a/b
b/b/a/a/a/b
b/b/a/a/b
b/b/b/a/a/b
__OUT__

test_oE 'complicated extendedglob ***/a/a/b' --extendedglob
printf '%s\n' ***/a/a/b
__IN__
a/a/a/a/a/a/b
a/a/a/a/a/b
a/a/a/a/b
a/a/a/a/b/a/a/a/a/b
a/a/a/a/b/a/a/a/b
a/a/a/a/b/a/a/b
a/a/a/a/b/a/b/a/a/b
a/a/a/a/b/b/a/a/a/b
a/a/a/a/b/b/a/a/b
a/a/a/a/b/b/b/a/a/b
a/a/a/b
a/a/a/b/a/a/b
a/a/a/b/b/a/a/a/a/b
a/a/a/b/b/a/a/a/b
a/a/a/b/b/a/a/b
a/a/a/b/b/a/b/a/a/b
a/a/a/b/b/b/a/a/a/b
a/a/a/b/b/b/a/a/b
a/a/a/b/b/b/b/a/a/b
a/a/b
a/a/b/a/a/a/b
a/a/b/a/a/b
a/a/b/a/b/a/a/a/a/b
a/a/b/a/b/a/a/a/b
a/a/b/a/b/a/a/b
a/a/b/a/b/a/b/a/a/b
a/a/b/a/b/b/a/a/a/b
a/a/b/a/b/b/a/a/b
a/a/b/a/b/b/b/a/a/b
a/a/b/b/a/a/b
a/a/b/b/b/a/a/a/a/b
a/a/b/b/b/a/a/a/b
a/a/b/b/b/a/a/b
a/a/b/b/b/a/b/a/a/b
a/a/b/b/b/b/a/a/a/b
a/a/b/b/b/b/a/a/b
a/a/b/b/b/b/b/a/a/b
a/b/a/a/a/a/b
a/b/a/a/a/b
a/b/a/a/b
a/b/a/a/b/a/a/a/a/b
a/b/a/a/b/a/a/a/b
a/b/a/a/b/a/a/b
a/b/a/a/b/a/b/a/a/b
a/b/a/a/b/b/a/a/a/b
a/b/a/a/b/b/a/a/b
a/b/a/a/b/b/b/a/a/b
a/b/a/b/a/a/b
a/b/a/b/b/a/a/a/a/b
a/b/a/b/b/a/a/a/b
a/b/a/b/b/a/a/b
a/b/a/b/b/a/b/a/a/b
a/b/a/b/b/b/a/a/a/b
a/b/a/b/b/b/a/a/b
a/b/a/b/b/b/b/a/a/b
a/b/b/a/a/a/b
a/b/b/a/a/b
a/b/b/a/b/a/a/a/a/b
a/b/b/a/b/a/a/a/b
a/b/b/a/b/a/a/b
a/b/b/a/b/a/b/a/a/b
a/b/b/a/b/b/a/a/a/b
a/b/b/a/b/b/a/a/b
a/b/b/a/b/b/b/a/a/b
a/b/b/b/a/a/b
a/b/b/b/b/a/a/a/a/b
a/b/b/b/b/a/a/a/b
a/b/b/b/b/a/a/b
a/b/b/b/b/a/b/a/a/b
a/b/b/b/b/b/a/a/a/b
a/b/b/b/b/b/a/a/b
a/b/b/b/b/b/b/a/a/b
b/a/a/a/a/a/a/a/a/b
b/a/a/a/a/a/a/a/b
b/a/a/a/a/a/a/b
b/a/a/a/a/a/b
b/a/a/a/a/a/b/a/a/b
b/a/a/a/a/b
b/a/a/a/a/b/a/a/a/b
b/a/a/a/a/b/a/a/b
b/a/a/a/a/b/b/a/a/b
b/a/a/a/b
b/a/a/b
b/a/a/b/a/a/a/a/a/b
b/a/a/b/a/a/a/a/b
b/a/a/b/a/a/a/b
b/a/a/b/a/a/b
b/a/a/b/a/a/b/a/a/b
b/a/a/b/a/b/a/a/a/b
b/a/a/b/a/b/a/a/b
b/a/a/b/a/b/b/a/a/b
b/a/b/a/a/a/a/a/a/b
b/a/b/a/a/a/a/a/b
b/a/b/a/a/a/a/b
b/a/b/a/a/a/b
b/a/b/a/a/a/b/a/a/b
b/a/b/a/a/b
b/a/b/a/a/b/a/a/a/b
b/a/b/a/a/b/a/a/b
b/a/b/a/a/b/b/a/a/b
b/a/b/b/a/a/a/a/a/b
b/a/b/b/a/a/a/a/b
b/a/b/b/a/a/a/b
b/a/b/b/a/a/b
b/a/b/b/a/a/b/a/a/b
b/a/b/b/a/b/a/a/a/b
b/a/b/b/a/b/a/a/b
b/a/b/b/a/b/b/a/a/b
b/b/a/a/a/a/a/a/a/b
b/b/a/a/a/a/a/a/b
b/b/a/a/a/a/a/b
b/b/a/a/a/a/b
b/b/a/a/a/a/b/a/a/b
b/b/a/a/a/b
b/b/a/a/a/b/a/a/a/b
b/b/a/a/a/b/a/a/b
b/b/a/a/a/b/b/a/a/b
b/b/a/a/b
b/b/a/b/a/a/a/a/a/b
b/b/a/b/a/a/a/a/b
b/b/a/b/a/a/a/b
b/b/a/b/a/a/b
b/b/a/b/a/a/b/a/a/b
b/b/a/b/a/b/a/a/a/b
b/b/a/b/a/b/a/a/b
b/b/a/b/a/b/b/a/a/b
b/b/b/a/a/a/a/a/a/b
b/b/b/a/a/a/a/a/b
b/b/b/a/a/a/a/b
b/b/b/a/a/a/b
b/b/b/a/a/a/b/a/a/b
b/b/b/a/a/b
b/b/b/a/a/b/a/a/a/b
b/b/b/a/a/b/a/a/b
b/b/b/a/a/b/b/a/a/b
b/b/b/b/a/a/a/a/a/b
b/b/b/b/a/a/a/a/b
b/b/b/b/a/a/a/b
b/b/b/b/a/a/b
b/b/b/b/a/a/b/a/a/b
b/b/b/b/a/b/a/a/a/b
b/b/b/b/a/b/a/a/b
b/b/b/b/a/b/b/a/a/b
__OUT__

test_oE 'complicated extendedglob a/**//**/b' --extendedglob
printf '%s\n' a/**//**/b
__IN__
a//a/a/b
a//a/b
a//a/b/b
a//b
a//b/a/b
a//b/b
a//b/b/b
a/a//a/b
a/a//b
a/a//b/b
a/a/a//b
a/a/b//b
a/b//a/b
a/b//b
a/b//b/b
a/b/a//b
a/b/b//b
__OUT__

test_oE 'complicated extendedglob **/a/**/b' --extendedglob
printf '%s\n' **/a/**/b
__IN__
a/a/a/a/a/a/a/b
a/a/a/a/a/a/b
a/a/a/a/a/a/b/b
a/a/a/a/a/b
a/a/a/a/a/b/a/b
a/a/a/a/a/b/b
a/a/a/a/a/b/b/b
a/a/a/a/b
a/a/a/a/b/a/a/b
a/a/a/a/b/a/b
a/a/a/a/b/a/b/b
a/a/a/a/b/b
a/a/a/a/b/b/a/b
a/a/a/a/b/b/b
a/a/a/a/b/b/b/b
a/a/a/b
a/a/b
a/a/b/a/a/a/a/b
a/a/b/a/a/a/b
a/a/b/a/a/a/b/b
a/a/b/a/a/b
a/a/b/a/a/b/a/b
a/a/b/a/a/b/b
a/a/b/a/a/b/b/b
a/a/b/a/b
a/a/b/a/b/a/a/b
a/a/b/a/b/a/b
a/a/b/a/b/a/b/b
a/a/b/a/b/b
a/a/b/a/b/b/a/b
a/a/b/a/b/b/b
a/a/b/a/b/b/b/b
a/a/b/b
a/b
a/b/a/a/a/a/a/b
a/b/a/a/a/a/b
a/b/a/a/a/a/b/b
a/b/a/a/a/b
a/b/a/a/a/b/a/b
a/b/a/a/a/b/b
a/b/a/a/a/b/b/b
a/b/a/a/b
a/b/a/a/b/a/a/b
a/b/a/a/b/a/b
a/b/a/a/b/a/b/b
a/b/a/a/b/b
a/b/a/a/b/b/a/b
a/b/a/a/b/b/b
a/b/a/a/b/b/b/b
a/b/a/b
a/b/b
a/b/b/a/a/a/a/b
a/b/b/a/a/a/b
a/b/b/a/a/a/b/b
a/b/b/a/a/b
a/b/b/a/a/b/a/b
a/b/b/a/a/b/b
a/b/b/a/a/b/b/b
a/b/b/a/b
a/b/b/a/b/a/a/b
a/b/b/a/b/a/b
a/b/b/a/b/a/b/b
a/b/b/a/b/b
a/b/b/a/b/b/a/b
a/b/b/a/b/b/b
a/b/b/a/b/b/b/b
a/b/b/b
b/a/a/a/a/a/a/b
b/a/a/a/a/a/b
b/a/a/a/a/a/b/b
b/a/a/a/a/b
b/a/a/a/a/b/a/b
b/a/a/a/a/b/b
b/a/a/a/a/b/b/b
b/a/a/a/b
b/a/a/a/b/a/a/b
b/a/a/a/b/a/b
b/a/a/a/b/a/b/b
b/a/a/a/b/b
b/a/a/a/b/b/a/b
b/a/a/a/b/b/b
b/a/a/a/b/b/b/b
b/a/a/b
b/a/b
b/a/b/a/a/a/a/b
b/a/b/a/a/a/b
b/a/b/a/a/a/b/b
b/a/b/a/a/b
b/a/b/a/a/b/a/b
b/a/b/a/a/b/b
b/a/b/a/a/b/b/b
b/a/b/a/b
b/a/b/a/b/a/a/b
b/a/b/a/b/a/b
b/a/b/a/b/a/b/b
b/a/b/a/b/b
b/a/b/a/b/b/a/b
b/a/b/a/b/b/b
b/a/b/a/b/b/b/b
b/a/b/b
b/b/a/a/a/a/a/b
b/b/a/a/a/a/b
b/b/a/a/a/a/b/b
b/b/a/a/a/b
b/b/a/a/a/b/a/b
b/b/a/a/a/b/b
b/b/a/a/a/b/b/b
b/b/a/a/b
b/b/a/a/b/a/a/b
b/b/a/a/b/a/b
b/b/a/a/b/a/b/b
b/b/a/a/b/b
b/b/a/a/b/b/a/b
b/b/a/a/b/b/b
b/b/a/a/b/b/b/b
b/b/a/b
b/b/b/a/a/a/a/b
b/b/b/a/a/a/b
b/b/b/a/a/a/b/b
b/b/b/a/a/b
b/b/b/a/a/b/a/b
b/b/b/a/a/b/b
b/b/b/a/a/b/b/b
b/b/b/a/b
b/b/b/a/b/a/a/b
b/b/b/a/b/a/b
b/b/b/a/b/a/b/b
b/b/b/a/b/b
b/b/b/a/b/b/a/b
b/b/b/a/b/b/b
b/b/b/a/b/b/b/b
__OUT__

test_oE 'complicated extendedglob **/a/b/a/b/**/a' --extendedglob
printf '%s\n' **/a/b/a/b/**/a
__IN__
a/a/a/a/b/a/b/a
a/a/a/b/a/b/a
a/a/a/b/a/b/a/a
a/a/a/b/a/b/b/a
a/a/b/a/b/a
a/a/b/a/b/a/a
a/a/b/a/b/a/a/a
a/a/b/a/b/a/b/a
a/a/b/a/b/b/a
a/a/b/a/b/b/a/a
a/a/b/a/b/b/b/a
a/b/a/a/b/a/b/a
a/b/a/b/a
a/b/a/b/a/a
a/b/a/b/a/a/a
a/b/a/b/a/a/a/a
a/b/a/b/a/a/b/a
a/b/a/b/a/b/a
a/b/a/b/a/b/a/a
a/b/a/b/a/b/b/a
a/b/a/b/b/a
a/b/a/b/b/a/a
a/b/a/b/b/a/a/a
a/b/a/b/b/a/b/a
a/b/a/b/b/b/a
a/b/a/b/b/b/a/a
a/b/a/b/b/b/b/a
a/b/b/a/b/a/b/a
b/a/a/a/b/a/b/a
b/a/a/b/a/b/a
b/a/a/b/a/b/a/a
b/a/a/b/a/b/b/a
b/a/b/a/b/a
b/a/b/a/b/a/a
b/a/b/a/b/a/a/a
b/a/b/a/b/a/b/a
b/a/b/a/b/b/a
b/a/b/a/b/b/a/a
b/a/b/a/b/b/b/a
b/b/a/a/b/a/b/a
b/b/a/b/a/b/a
b/b/a/b/a/b/a/a
b/b/a/b/a/b/b/a
b/b/b/a/b/a/b/a
__OUT__

)

mkdir nullglob
>nullglob/xxx

(
setup -d
setup 'cd nullglob'

test_oE 'nullglob on: effect' --nullglob
bracket n*ll f[o/b]r f?o/b*r x*x
__IN__
[f[o/b]r][xxx]
__OUT__

test_oE 'nullglob off: effect' --nonullglob
bracket n*ll f[o/b]r f?o/b*r x*x
__IN__
[n*ll][f[o/b]r][f?o/b*r][xxx]
__OUT__

)

# vim: set ft=sh ts=8 sts=4 sw=4 et:
