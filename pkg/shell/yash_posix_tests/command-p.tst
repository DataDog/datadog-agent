# command-p.tst: test of the command built-in for any POSIX-compliant shell

posix="true"

test_o 'redirection error on special built-in does not kill shell'
command : <_no_such_file_
echo reached
__IN__
reached
__OUT__

test_o 'dot script not found does not kill shell'
command . ./_no_such_file_
echo reached
__IN__
reached
__OUT__

test_o 'assignment on special built-in is temporary'
a=a
a=b command :
echo $a
__IN__
a
__OUT__

test_OE -e 0 'command ignores function (mandatory built-in)'
alias () { false; }
command alias
__IN__

test_E -e 0 'command ignores function (substitutive built-in)'
echo () { false; }
command echo
__IN__

test_OE -e 0 'command ignores function (external command)'
cat () { false; }
command cat </dev/null
__IN__

test_oE -e 0 'command exec retains redirection'
command exec 3<<\__END__
here
__END__
cat <&3
__IN__
here
__OUT__

test_oE 'effect on environment'
command read a <<\__END__
foo
__END__
echo $a
__IN__
foo
__OUT__

test_o -e 0 'executing with standard path'
PATH=
command -p echo foo bar | command -p cat
__IN__
foo bar
__OUT__

(
setup 'set -e'

test_oE -e 0 'describing reserved word ! (-v)'
command -v !
__IN__
!
__OUT__

test_oE -e 0 'describing reserved word { (-v)'
command -v {
__IN__
{
__OUT__

test_oE -e 0 'describing reserved word } (-v)'
command -v }
__IN__
}
__OUT__

test_oE -e 0 'describing reserved word case (-v)'
command -v case
__IN__
case
__OUT__

test_oE -e 0 'describing reserved word do (-v)'
command -v do
__IN__
do
__OUT__

test_oE -e 0 'describing reserved word done (-v)'
command -v done
__IN__
done
__OUT__

test_oE -e 0 'describing reserved word elif (-v)'
command -v elif
__IN__
elif
__OUT__

test_oE -e 0 'describing reserved word else (-v)'
command -v else
__IN__
else
__OUT__

test_oE -e 0 'describing reserved word esac (-v)'
command -v esac
__IN__
esac
__OUT__

test_oE -e 0 'describing reserved word fi (-v)'
command -v fi
__IN__
fi
__OUT__

test_oE -e 0 'describing reserved word for (-v)'
command -v for
__IN__
for
__OUT__

test_oE -e 0 'describing reserved word if (-v)'
command -v if
__IN__
if
__OUT__

test_oE -e 0 'describing reserved word in (-v)'
command -v in
__IN__
in
__OUT__

test_oE -e 0 'describing reserved word then (-v)'
command -v then
__IN__
then
__OUT__

test_oE -e 0 'describing reserved word until (-v)'
command -v until
__IN__
until
__OUT__

test_oE -e 0 'describing reserved word while (-v)'
command -v while
__IN__
while
__OUT__

test_E -e 0 'describing reserved word (-V)'
command -V !
__IN__

test_oE -e 0 'describing special built-in (-v)'
command -v :
__IN__
:
__OUT__

test_E -e 0 'describing special built-in (-V)'
command -V :
__IN__

test_x -e 0 'exit status of describing non-special built-in (-v)'
command -v echo
__IN__

test_x -e 0 'exit status of describing non-special built-in (-V)'
command -V echo
__IN__

test_E -e 0 'output of describing non-special built-in (-v)'
command -v echo | grep '^/'
__IN__

test_x -e 0 'output of describing non-special built-in (-V)'
command -V echo | grep -F "$(command -v echo)"
__IN__

test_x -e 0 'exit status of describing external command (-v, no slash)'
command -v cat
__IN__

test_x -e 0 'exit status of describing external command (-V, no slash)'
command -V cat
__IN__

test_E -e 0 'output of describing external command (-v, no slash)'
command -v cat | grep '^/'
__IN__

test_E -e 0 'output of describing external command (-V, no slash)'
command -V cat | grep -F "$(command -v cat)"
__IN__

>foo
chmod a+x foo

test_x -e 0 'exit status of describing external command (-v, with slash)'
command -v ./foo
__IN__

test_x -e 0 'exit status of describing external command (-V, with slash)'
command -V ./foo
__IN__

test_E -e 0 'output of describing external command (-v, with slash)'
command -v ./foo | grep '^/' | grep '/foo$'
__IN__

test_E -e 0 'output of describing external command (-V, with slash)'
command -V ./foo | grep -F "$(command -v ./foo)"
__IN__

test_oE -e 0 'describing function (-v)'
cat() { :; }
command -v cat
__IN__
cat
__OUT__

test_E -e 0 'describing function (-V)'
cat() { :; }
command -V cat
__IN__

test_oE -e 0 'describing alias (-v)'
alias abc='echo ABC'
command="$(command -v abc)"
unalias abc
eval "$command"
abc
__IN__
ABC
__OUT__

test_OE -e 0 'describing alias (-V)'
alias abc=xyz
d="$(command -V abc)"
case "$d" in
    (*abc*xyz*|*xyz*abc*) # expected output contains alias name and value
        ;;
    (*)
        printf '%s\n' "$d" # print non-conforming result
        ;;
esac
__IN__

test_OE -e n 'describing non-existent command (-v)'
PATH=
command -v _no_such_command_
__IN__

test_x -e n 'describing non-existent command (-V)'
PATH=
command -V _no_such_command_
__IN__

test_x -e 0 'describing external command with standard path (-v)'
PATH=
command -pv cat
__IN__

test_x -e 0 'describing external command with standard path (-V)'
PATH=
command -pV cat
__IN__

)

test_O -d -e 127 'executing non-existent command'
command ./_no_such_command_
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
