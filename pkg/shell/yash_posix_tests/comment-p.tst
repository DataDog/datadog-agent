# comment-p.tst: test of comments for any POSIX-compliant shell

posix="true"

test_OE 'comment without command'
#

# foo

#	bar
 #
	#

##foo
###
__IN__

test_oE 'comment ending with backslash'
# \
echo foo
__IN__
foo
__OUT__

test_oE 'comment in simple command'
v=abc sh -c 'echo $v "$@"' # 0 1 2 3
echo 123 # 456 # 789; echo xyz
</dev/null # < foo
__IN__
abc
123
__OUT__

test_oE 'hash sign in word'
v=abc#def
echo 123#456 $v
echo {# #"
__IN__
123#456 abc#def
{#
__OUT__

test_oE 'comment in pipeline'
echo foo |###
cat #|:
__IN__
foo
__OUT__

test_oE 'comment in and-or list'
echo foo &&###
echo bar ||###
echo baz
__IN__
foo
bar
__OUT__

test_oE 'comment after (a)synchronous list'
echo foo&###
wait;###
__IN__
foo
__OUT__

test_oE 'comment in grouping'
{ ###
    echo foo ###
} ###
__IN__
foo
__OUT__

test_oE 'comment in subshell'
(###
    echo foo ###
)###
__IN__
foo
__OUT__

test_oE 'comment in for loop'
for v ###
in 1 2 3 ###
do ###
    echo $v ###
done </dev/null ###
__IN__
1
2
3
__OUT__

test_oE 'comment in case statement'
case 1 ###
in ### esac
### esac
0)### esac
###
;;###
###
(1|2)# esac
    echo foo # esac
    echo bar #;;
    ;;###
###
esac </dev/null ###
__IN__
foo
bar
__OUT__

test_oE 'comment in if statement'
if ###
    ###
    echo foo
    false
    ###
then ###
    ###
    :
    ###
elif ###
    ###
    echo bar
    ###
then ###
    ###
    echo baz
    ###
else ###
    ###
    echo qux
    ###
fi </dev/null ###
__IN__
foo
bar
baz
__OUT__

test_oE 'comment in while loop'
while ###
    ###
    ! echo foo
    ###
do ###
    ###
    echo not reached
    ###
done </dev/null ###
__IN__
foo
__OUT__

test_oE 'comment in until loop'
until ###
    ###
    echo foo
    ###
do ###
    ###
    echo not reached
    ###
done </dev/null ###
__IN__
foo
__OUT__

test_OE 'comment in function definition'
func()###
###
{ ###
    :
} ###
__IN__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
