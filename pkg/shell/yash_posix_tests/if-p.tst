# if-p.tst: test of if conditional construct for any POSIX-compliant shell

posix="true"

test_oE 'execution path of if, true'
if echo foo; then echo bar; fi
__IN__
foo
bar
__OUT__

test_oE 'execution path of if, false'
if ! echo foo; then echo bar; fi
__IN__
foo
__OUT__

test_oE 'execution path of if-else, true'
if echo foo; then echo bar; else echo baz; fi
__IN__
foo
bar
__OUT__

test_oE 'execution path of if-else, false'
if ! echo foo; then echo bar; else echo baz; fi
__IN__
foo
baz
__OUT__

test_oE 'execution path of if-elif, true'
if echo 1; then echo 2; elif echo 3; then echo 4; fi
__IN__
1
2
__OUT__

test_oE 'execution path of if-elif, false-true'
if ! echo 1; then echo 2; elif echo 3; then echo 4; fi
__IN__
1
3
4
__OUT__

test_oE 'execution path of if-elif, false-false'
if ! echo 1; then echo 2; elif ! echo 3; then echo 4; fi
__IN__
1
3
__OUT__

test_oE 'execution path of if-elif-else, true'
if echo 1; then echo 2; elif echo 3; then echo 4; else echo 5; fi
__IN__
1
2
__OUT__

test_oE 'execution path of if-elif-else, false-true'
if ! echo 1; then echo 2; elif echo 3; then echo 4; else echo 5; fi
__IN__
1
3
4
__OUT__

test_oE 'execution path of if-elif-else, false-false'
if ! echo 1; then echo 2; elif ! echo 3; then echo 4; else echo 5; fi
__IN__
1
3
5
__OUT__

test_oE 'execution path of if-elif-elif, true'
if echo 1; then echo 2; elif echo 3; then echo 4; elif echo 5; then echo 6; fi
__IN__
1
2
__OUT__

test_oE 'execution path of if-elif-elif, false-true'
if ! echo 1; then echo 2; elif echo 3; then echo 4; elif echo 5; then echo 6; fi
__IN__
1
3
4
__OUT__

test_oE 'execution path of if-elif-elif, false-false-true'
if ! echo 1; then echo 2; elif ! echo 3; then echo 4; elif echo 5; then echo 6; fi
__IN__
1
3
5
6
__OUT__

test_oE 'execution path of if-elif-elif, false-false-false'
if ! echo 1; then echo 2; elif ! echo 3; then echo 4; elif ! echo 5; then echo 6; fi
__IN__
1
3
5
__OUT__

test_oE 'execution path of if-elif-elif-else, true'
if echo 1; then echo 2; elif echo 3; then echo 4; elif echo 5; then echo 6; else echo 7; fi
__IN__
1
2
__OUT__

test_oE 'execution path of if-elif-elif-else, false-true'
if ! echo 1; then echo 2; elif echo 3; then echo 4; elif echo 5; then echo 6; else echo 7; fi
__IN__
1
3
4
__OUT__

test_oE 'execution path of if-elif-elif-else, false-false-true'
if ! echo 1; then echo 2; elif ! echo 3; then echo 4; elif echo 5; then echo 6; else echo 7; fi
__IN__
1
3
5
6
__OUT__

test_oE 'execution path of if-elif-elif-else, false-false-false'
if ! echo 1; then echo 2; elif ! echo 3; then echo 4; elif ! echo 5; then echo 6; else echo 7; fi
__IN__
1
3
5
7
__OUT__

(
setup <<\__END__
\unalias \x
x() { return $1; }
__END__

test_x -e 0 'exit status of if, true-true'
if x 0; then x 0; fi
__IN__

test_x -e 1 'exit status of if, true-false'
if x 0; then x 1; fi
__IN__

test_x -e 0 'exit status of if, false'
if x 1; then x 2; fi
__IN__

test_x -e 0 'exit status of if-else, true-true'
if x 0; then x 0; else x 1; fi
__IN__

test_x -e 1 'exit status of if-else, true-false'
if x 0; then x 1; else x 2; fi
__IN__

test_x -e 0 'exit status of if-else, false-true'
if x 1; then x 2; else x 0; fi
__IN__

test_x -e 2 'exit status of if-else, false-false'
if x 1; then x 0; else x 2; fi
__IN__

test_x -e 0 'exit status of if-elif, true-true'
if x 0; then x 0; elif x 1; then x 2; fi
__IN__

test_x -e 1 'exit status of if-elif, true-false'
if x 0; then x 1; elif x 2; then x 3; fi
__IN__

test_x -e 0 'exit status of if-elif, false-true-true'
if x 1; then x 2; elif x 0; then x 0; fi
__IN__

test_x -e 3 'exit status of if-elif, false-true-false'
if x 1; then x 2; elif x 0; then x 3; fi
__IN__

test_x -e 0 'exit status of if-elif-elif-else, true-true'
if x 0; then x 0; elif x 1; then x 2; elif x 3; then x 4; else x 5; fi
__IN__

test_x -e 11 'exit status of if-elif-elif-else, true-false'
if x 0; then x 11; elif x 1; then x 2; elif x 3; then x 4; else x 5; fi
__IN__

test_x -e 0 'exit status of if-elif-elif-else, false-true-true'
if x 1; then x 2; elif x 0; then x 0; elif x 3; then x 4; else x 5; fi
__IN__

test_x -e 13 'exit status of if-elif-elif-else, false-true-false'
if x 1; then x 2; elif x 0; then x 13; elif x 3; then x 4; else x 5; fi
__IN__

test_x -e 0 'exit status of if-elif-elif-else, false-false-true-true'
if x 1; then x 2; elif x 3; then x 4; elif x 0; then x 0; else x 5; fi
__IN__

test_x -e 5 'exit status of if-elif-elif-else, false-false-true-false'
if x 1; then x 2; elif x 3; then x 4; elif x 0; then x 5; else x 6; fi
__IN__

test_x -e 0 'exit status of if-elif-elif-else, false-false-false-true'
if x 1; then x 2; elif x 3; then x 4; elif x 5; then x 6; else x 0; fi
__IN__

test_x -e 7 'exit status of if-elif-elif-else, false-false-false-false'
if x 1; then x 2; elif x 3; then x 4; elif x 5; then x 6; else x 7; fi
__IN__

)

test_oE 'linebreak after if'
if

    echo foo;then echo bar;fi
__IN__
foo
bar
__OUT__

test_oE 'linebreak before then (after if)'
if echo foo

    then echo bar;fi
__IN__
foo
bar
__OUT__

test_oE 'linebreak after then (after if)'
if echo foo;then
    
    echo bar;fi
__IN__
foo
bar
__OUT__

test_oE 'linebreak before fi (after then)'
if echo foo;then echo bar

    fi
__IN__
foo
bar
__OUT__

test_oE 'linebreak before elif'
if ! echo foo;then echo bar

    elif echo baz;then echo qux;fi
__IN__
foo
baz
qux
__OUT__

test_oE 'linebreak after elif'
if ! echo foo;then echo bar;elif
    
    echo baz;then echo qux;fi
__IN__
foo
baz
qux
__OUT__

test_oE 'linebreak before then (after elif)'
if ! echo foo;then echo bar;elif echo baz

    then echo qux;fi
__IN__
foo
baz
qux
__OUT__

test_oE 'linebreak after then (after elif)'
if ! echo foo;then echo bar;elif echo baz;then
    
    echo qux;fi
__IN__
foo
baz
qux
__OUT__

test_oE 'linebreak before else'
if ! echo foo;then echo bar

    else echo baz;fi
__IN__
foo
baz
__OUT__

test_oE 'linebreak after else'
if ! echo foo;then echo bar;else
    
    echo baz;fi
__IN__
foo
baz
__OUT__

test_oE 'linebreak before fi (after else)'
if ! echo foo;then echo bar;else echo baz

    fi
__IN__
foo
baz
__OUT__

test_oE 'command ending with asynchronous command (after if)'
if echo foo&then wait;fi
__IN__
foo
__OUT__

test_oE 'command ending with asynchronous command (after then)'
if echo foo;then echo bar&fi;wait
__IN__
foo
bar
__OUT__

test_oE 'command ending with asynchronous command (after elif)'
if ! echo foo;then echo bar;elif echo baz&then wait;fi
__IN__
foo
baz
__OUT__

test_oE 'command ending with asynchronous command (after else)'
if ! echo foo;then echo bar;elif ! echo baz;then echo qux;else echo quux;fi;wait
__IN__
foo
baz
quux
__OUT__

test_oE 'more than one inner command'
if echo 1; echo 2
    echo 3; ! echo 4; then echo x1; echo x2
    echo x3; echo x4; elif echo 5; echo 6
    echo 7; echo 8; then echo 9; echo 10
    echo 11; echo 12; else echo x5; echo x6
    echo x7; echo x8; fi
__IN__
1
2
3
4
5
6
7
8
9
10
11
12
__OUT__

test_oE 'nest between if and then'
if if true; then echo foo; fi then echo bar; fi
__IN__
foo
bar
__OUT__

test_oE 'nest between then and fi'
if echo foo; then if true; then echo bar; fi fi
__IN__
foo
bar
__OUT__

test_oE 'nest between then and elif'
if echo foo; then if echo bar; then true; fi elif echo baz; then echo qux; fi
__IN__
foo
bar
__OUT__

test_oE 'nest between elif and then'
if echo foo; then echo bar; elif if true; then echo baz; fi then echo qux; fi
__IN__
foo
bar
__OUT__

test_oE 'nest between then and else'
if echo foo; then if echo bar; then true; fi else echo baz; fi
__IN__
foo
bar
__OUT__

test_oE 'nest between else and if'
if ! echo foo; then echo bar; else if echo baz; then echo qux; fi fi
__IN__
foo
baz
qux
__OUT__

test_oE 'redirection on if'
if echo foo
then echo bar
else echo baz
fi >redir_out
cat redir_out
__IN__
foo
bar
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
