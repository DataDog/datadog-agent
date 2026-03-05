# while-p.tst: test of while loop for any POSIX-compliant shell

posix="true"

test_oE 'execution path of 0-round loop'
i=0
while [ $((i=i+1)) -le 0 ];do echo $i;done
echo done $i
__IN__
done 1
__OUT__

test_oE 'execution path of 1-round loop'
i=0
while [ $((i=i+1)) -le 1 ];do echo $i;done
echo done $i
__IN__
1
done 2
__OUT__

test_oE 'execution path of 2-round loop'
i=0
while [ $((i=i+1)) -le 2 ];do echo $i;done
echo done $i
__IN__
1
2
done 3
__OUT__

(
setup <<\__END__
\unalias \x
x() { return $1; }
__END__

test_x -e 0 'exit status of 0-round loop'
while false;do :;done
__IN__

test_x -e 1 'exit status of 1-round loop'
i=0
while [ $((i=i+1)) -le 1 ];do x $i;done
__IN__

test_x -e 2 'exit status of 2-round loop'
i=0
while [ $((i=i+1)) -le 2 ];do x $i;done
__IN__

)

test_oE 'linebreak after while'
i=0
while
    
    [ $((i=i+1)) -le 2 ];do echo $i;done
__IN__
1
2
__OUT__

test_oE 'linebreak before do'
i=0
while [ $((i=i+1)) -le 2 ]

    do echo $i;done
__IN__
1
2
__OUT__

test_oE 'linebreak after do'
i=0
while [ $((i=i+1)) -le 2 ];do
    
    echo $i;done
__IN__
1
2
__OUT__

test_oE 'linebreak before done'
i=0
while [ $((i=i+1)) -le 2 ];do echo $i

    done
__IN__
1
2
__OUT__

test_oE 'command ending with asynchronous command (condition)'
while echo foo&do break;done;wait
__IN__
foo
__OUT__

test_oE 'command ending with asynchronous command (body)'
i=0
while [ $((i=i+1)) -le 1 ];do echo $i&done
wait
__IN__
1
__OUT__

test_oE 'more than one inner command'
i=0
while i=$((i+1)); [ $i -le 2 ];do echo $i;echo -;done
__IN__
1
-
2
-
__OUT__

test_oE 'nest between while and do'
i=0
while { [ $((i=i+1)) -le 1 ]; } do echo $i;done
__IN__
1
__OUT__

test_oE 'nest between do and done'
i=0
while [ $((i=i+1)) -le 1 ]; do { echo $i;} done
__IN__
1
__OUT__

test_oE 'redirection on while loop'
i=0
while echo -;[ $((i=i+1)) -le 1 ];do echo $i;done >redir_out
cat redir_out
__IN__
-
1
-
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
