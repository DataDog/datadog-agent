# for-p.tst: test of for loop for any POSIX-compliant shell

posix="true"

test_OE 'default words, no positional parameters'
for i do
    echo not reached
done
__IN__

test_oE 'default words, one positional parameter' -s A
for i do
    echo $i
done
__IN__
A
__OUT__

test_oE 'default words, one positional parameter' -s A ' B  B '
for word do
    echo "$word"
done
__IN__
A
 B  B 
__OUT__

test_OE 'explicit words, no words, newline-separated do'
for i in
do
    echo not reached
done
__IN__

test_oE 'explicit words, one word, newline-separated do'
for	i	in	do
do
    echo $i
done
__IN__
do
__OUT__

test_oE 'explicit words, two words, newline-separated do'
for	word	in	do done
do
    echo $word
done
__IN__
do
done
__OUT__

test_oE 'expansion of words'
HOME=/home
for i in ~ $HOME $(echo foo) $((1+2))
do
    echo $i
done
for i in $(echo foo bar)
do
    echo $i
done
for i in
do
    echo $i
done
__IN__
/home
/home
foo
3
foo
bar
__OUT__

test_oE 'words are not treated as assignments'
v=foo
for i in v=bar; do echo $i $v; done
__IN__
v=bar foo
__OUT__

test_oE 'semicolon-separated commands'
for v in 1 2; do echo $v; done
__IN__
1
2
__OUT__

test_oE 'commands ending with an asynchronous command'
for v in 1 2; do true; echo& done
wait
__IN__


__OUT__

test_oE 'for as variable name' -s foo
for for do echo $for; done
__IN__
foo
__OUT__

test_oE 'do as variable name' -s foo
for do do echo $do; done
__IN__
foo
__OUT__

test_oE 'in as variable name'
for in in foo; do echo $in; done
__IN__
foo
__OUT__

test_oE 'in as word'
for i in in; do echo $i; done
__IN__
in
__OUT__

test_oE -e 0 'default words, do separated by semicolon' -s A B
for i; do echo $i; done
__IN__
A
B
__OUT__

test_oE -e 0 'default words, do separated by semicolon and newlines' -s A B
for i ;

do echo $i; done
__IN__
A
B
__OUT__

test_x -e 0 'exit status with no words'
false
for i do
    false
done
__IN__

test_x -e 3 'exit status with some words'
for x in 1 2 3
do
    (exit $x)
done
__IN__

test_o 'redirection on for loop'
for i in a b c; do read j; echo $i $j; done >redir_out <<END
1
2
3
END
cat redir_out
__IN__
a 1
b 2
c 3
__OUT__

test_o 'iteration variable is global'
unset -v i
fn() { for i in a b c; do : ; done; }
fn
echo "${i-UNSET}"
__IN__
c
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
