# cmdprint-y.tst: yash-specific test of command printing

mkfifo fifo

# Commands used in testcase_single must perform exactly one "cat fifo" so that
# the background job finishes after "jobs" prints the job status.
testcase_single() {
    testcase "$@" 3<<__IN__ 5<&- 4<<__OUT__
$(cat <&3) &
jobs
>fifo
__IN__
[1] + Running              $(cat <&4)
__OUT__
}

testcase_multi() {
    testcase "$@" 3<<__IN__ 5<&- 4<<__OUT__
eval "\$(
f()
$(cat <&3)
typeset -fp f
)"
typeset -fp f
__IN__
f()
$(cat <&4)
__OUT__
}

alias test_single='testcase_single "$LINENO" 3<<\__IN__ 4<<\__OUT__ 5<&-'
alias test_multi='testcase_multi "$LINENO" 3<<\__IN__ 4<<\__OUT__ 5<&-'

test_single 'one simple command, single line'
cat fifo
__IN__
cat fifo
__OUT__

test_multi 'one simple command, multi-line'
{ echo; }
typeset -fp f
__IN__
{
   echo
}
__OUT__

test_single 'many and-or lists, ending synchronously, single line'
{
    cat fifo; exit ; foo &
    bar& ls
    ls -l;
}
__IN__
{ cat fifo; exit; foo& bar& ls; ls -l; }
__OUT__

test_single 'many and-or lists, ending asynchronously, single line'
{
    :& true &
    cat fifo; exit
    foo &
    bar&
}
__IN__
{ :& true& cat fifo; exit; foo& bar& }
__OUT__

test_multi 'many and-or lists, multi-line'
{ echo ; echo 1 ; foo & bar & ls ; ls -l; }
typeset -fp f
__IN__
{
   echo
   echo 1
   foo&
   bar&
   ls
   ls -l
}
__OUT__

test_single 'many pipelines, single line'
cat fifo && : || echo not reached
__IN__
cat fifo && : || echo not reached
__OUT__

test_multi 'many pipelines, multi-line'
{ cat fifo || echo not reached && :; }
__IN__
{
   cat fifo ||
   echo not reached &&
   :
}
__OUT__

test_single 'many commands, single line'
cat fifo | cat - | cat
__IN__
cat fifo | cat - | cat
__OUT__

test_multi 'many commands, multi-line'
{ echo | cat - | cat; }
__IN__
{
   echo | cat - | cat
}
__OUT__

test_single 'negated pipeline, single line'
! cat fifo | cat - | cat
__IN__
! cat fifo | cat - | cat
__OUT__

test_multi 'negated pipeline, multi-line'
{ ! echo | cat - | cat; }
__IN__
{
   ! echo | cat - | cat
}
__OUT__

# Non-empty grouping is tested in other tests above.

test_single 'grouping, w/o commands, single line'
{ } && cat fifo
__IN__
{ } && cat fifo
__OUT__

test_multi 'grouping, w/o commands, multi-line'
{ }
__IN__
{
}
__OUT__

test_single 'subshell, w/ single command, ending synchronously, single line'
(cat fifo)
__IN__
(cat fifo)
__OUT__

test_single 'subshell, w/ many commands, ending asynchronously, single line'
(cat fifo; :&)
__IN__
(cat fifo; :&)
__OUT__

test_multi 'subshell, w/ simple command, ending synchronously, multi-line'
(cat fifo)
__IN__
(cat fifo)
__OUT__

test_multi 'subshell, w/ many commands, ending asynchronously, multi-line'
(:; cat fifo; :&)
__IN__
(:
   cat fifo
   :&)
__OUT__

test_single 'subshell, w/o commands, single line'
() && cat fifo
__IN__
() && cat fifo
__OUT__

test_single 'if command, w/o elif, w/o else, single line'
if :& :; then cat fifo; fi
__IN__
if :& :; then cat fifo; fi
__OUT__

test_multi 'if command, w/o elif, w/o else, multi-line'
if
    :&
    :
then
    cat fifo
fi
__IN__
if :&
   :
then
   cat fifo
fi
__OUT__

test_single 'if command, w/ elif, w/o else, single line'
if :& :; then cat fifo; :& elif foo; then :; elif bar& then :& fi
__IN__
if :& :; then cat fifo; :& elif foo; then :; elif bar& then :& fi
__OUT__

test_multi 'if command, w/ elif, w/o else, multi-line'
if [ ]; then foo& elif 1; then 2; elif a& b; then c& fi
__IN__
if [ ]
then
   foo&
elif 1
then
   2
elif a&
   b
then
   c&
fi
__OUT__

test_single 'if command, w/o elif, w/ else, single line'
if :& :; then cat fifo; else echo not reached; fi
__IN__
if :& :; then cat fifo; else echo not reached; fi
__OUT__

test_multi 'if command, w/o elif, w/ else, multi-line'
if :& :; then
    cat fifo
else
    echo not reached
fi
__IN__
if :&
   :
then
   cat fifo
else
   echo not reached
fi
__OUT__

test_single 'if command, w/ elif, w/ else, single line'
if :; then cat fifo; elif foo& then :; else bar& fi
__IN__
if :; then cat fifo; elif foo& then :; else bar& fi
__OUT__

test_multi 'if command, w/ elif, w/ else, multi-line'
if :; then cat fifo; elif foo& then :; else bar& fi
__IN__
if :
then
   cat fifo
elif foo&
then
   :
else
   bar&
fi
__OUT__

test_single 'if command, w/o commands, single line'
if then elif then elif then else fi && cat fifo
__IN__
if then elif then elif then else fi && cat fifo
__OUT__

test_multi 'if command, w/o commands, multi-line'
if then elif then elif then else fi
__IN__
if then
elif then
elif then
else
fi
__OUT__

test_single 'for command, w/o in, single line'
set 1 && for i do cat fifo; done
__IN__
set 1 && for i do cat fifo; done
__OUT__

test_multi 'for command, w/o in, multi-line'
for i do cat fifo; done
__IN__
for i do
   cat fifo
done
__OUT__

test_single 'for command, w/ in, w/o words, single line'
{ for i in; do echo not reached; done; cat fifo; }
__IN__
{ for i in; do echo not reached; done; cat fifo; }
__OUT__

test_multi 'for command, w/ in, w/o words, multi-line'
for i in; do echo not reached; done
__IN__
for i in
do
   echo not reached
done
__OUT__

test_single 'for command, w/ in, w/ many words, single line'
for i in 1 2 3; do cat fifo; break; :& done
__IN__
for i in 1 2 3; do cat fifo; break; :& done
__OUT__

test_multi 'for command, w/ in, w/ many words, multi-line'
for i in 1 2 3; do cat fifo; :; :& done
__IN__
for i in 1 2 3
do
   cat fifo
   :
   :&
done
__OUT__

test_single 'for command, w/ commands, single line'
for i do done && cat fifo
__IN__
for i do done && cat fifo
__OUT__

test_multi 'for command, w/ commands, multi-line'
for i do done
__IN__
for i do
done
__OUT__

test_single 'while command, w/ single command condition, single line'
while :; do cat fifo; break; done
__IN__
while :; do cat fifo; break; done
__OUT__

test_multi 'while command, w/ single command condition, multi-line'
while :; do foo; done
__IN__
while :
do
   foo
done
__OUT__

test_single 'while command, w/ many command condition, single line'
while cat fifo; break; :& do foo; bar& done
__IN__
while cat fifo; break; :& do foo; bar& done
__OUT__

test_multi 'while command, w/ many command condition, multi-line'
while cat fifo; break; :& do foo; bar& done
__IN__
while cat fifo
   break
   :&
do
   foo
   bar&
done
__OUT__

test_single 'while command, w/o commands, single line'
cat fifo || exit || while do done
__IN__
cat fifo || exit || while do done
__OUT__

test_multi 'while command, w/o commands, multi-line'
while do done
__IN__
while do
done
__OUT__

test_single 'case command, w/o case items, single line'
case i in esac && cat fifo
__IN__
case i in esac && cat fifo
__OUT__

test_multi 'case command, w/o case items, multi-line'
case i in esac
__IN__
case i in
esac
__OUT__

test_single 'case command, w/ case items, single line'
case i in (i) cat fifo;; (j) foo& bar;; (k|l|m) ;; (n) :& esac
__IN__
case i in (i) cat fifo ;; (j) foo& bar ;; (k | l | m) ;; (n) :& ;; esac
__OUT__

test_multi 'case command, w/ case items, multi-line'
case i in (i) cat fifo;; (j) foo& bar;; (k|l|m) ;; (n) :& esac
__IN__
case i in
   (i)
      cat fifo
      ;;
   (j)
      foo&
      bar
      ;;
   (k | l | m)
      ;;
   (n)
      :&
      ;;
esac
__OUT__

test_single 'case command, terminators, single line'
case 1 in (1) cat fifo;& (2) ;| (3) ./oops&;;& esac
__IN__
case 1 in (1) cat fifo ;& (2) ;| (3) ./oops& ;| esac
__OUT__

test_multi 'case command, terminators, multi-line'
case 1 in (1) cat fifo;& (2) ;| (3) ./oops&;;& esac
__IN__
case 1 in
   (1)
      cat fifo
      ;&
   (2)
      ;|
   (3)
      ./oops&
      ;|
esac
__OUT__

(
if ! testee -c 'command -v [[' >/dev/null; then
    skip="true"
fi

test_multi 'double bracket, string primary'
[[ "foo" ]]
__IN__
[[ "foo" ]]
__OUT__

test_multi 'double bracket, unary/binary primaries'
[[ -n foo || 0 -eq '1' || a = a || x < y ]]
__IN__
[[ -n foo || 0 -eq '1' || a = a || x < y ]]
__OUT__

test_multi 'double bracket, disjunction in disjunction'
[[ (a || b) || (c || d) ]]
__IN__
[[ a || b || c || d ]]
__OUT__

test_multi 'double bracket, conjunction in disjunction'
[[ (a && b) || (c && d) ]]
__IN__
[[ a && b || c && d ]]
__OUT__

test_multi 'double bracket, negation in disjunction'
[[ (! a) || (! b) ]]
__IN__
[[ ! a || ! b ]]
__OUT__

test_multi 'double bracket, disjunction in conjunction'
[[ (a || b) && (c || d) ]]
__IN__
[[ ( a || b ) && ( c || d ) ]]
__OUT__

test_multi 'double bracket, conjunction in conjunction'
[[ (a && b) && (c && d) ]]
__IN__
[[ a && b && c && d ]]
__OUT__

test_multi 'double bracket, negation in conjunction'
[[ (! a) && (! b) ]]
__IN__
[[ ! a && ! b ]]
__OUT__

test_multi 'double bracket, disjunction in negation'
[[ ! (a || b) ]]
__IN__
[[ ! ( a || b ) ]]
__OUT__

test_multi 'double bracket, conjunction in negation'
[[ ! (a && b) ]]
__IN__
[[ ! ( a && b ) ]]
__OUT__

test_multi 'double bracket, negation in negation'
[[ ! (! a) ]]
__IN__
[[ ! ! a ]]
__OUT__

)

test_single 'function definition, POSIX name, single line'
f() { :; } >/dev/null && cat fifo
__IN__
f() { :; } 1>/dev/null && cat fifo
__OUT__

# POSIX-name multi-line function definition is tested in all testcase_multi
# test cases.

test_single 'function definition, non-POSIX name, single line'
function "${a-f}" { :; } && cat fifo
__IN__
function "${a-f}"() { :; } && cat fifo
__OUT__

test_multi 'function definition, non-POSIX name, multi-line'
{
    function "${a-f}" { :; }
}
__IN__
{
   function "${a-f}"()
   {
      :
   }
}
__OUT__

test_multi 'scalar assignment'
{ foo= bar=BAR; }
__IN__
{
   foo= bar=BAR
}
__OUT__

test_multi 'array assignment'
{ foo=() bar=(1 $2 3); }
__IN__
{
   foo=() bar=(1 ${2} 3)
}
__OUT__

test_multi 'single-line redirections'
{ <f >g 2>|h 10>>i <>j <&1 >&2 >>|"3" <<<here\ string; }
__IN__
{
   0<f 1>g 2>|h 10>>i 0<>j 0<&1 1>&2 1>>|"3" 0<<<here\ string
}
__OUT__

test_multi 'keyword in simple command'
{ foo=bar if then fi; >/dev/null do do; }
__IN__
{
   foo=bar if then fi
   \do do 1>/dev/null
}
__OUT__

test_single 'here-documents, single line'
{
    cat fifo <<END 4<<\END <<-EOF
$1
END
$1
END
		    foo
	EOF
}
__IN__
{ cat fifo 0<<END 4<<\END 0<<-EOF; }
__OUT__

test_multi 'here-documents, multi-line'
{ <<END 4<<\END <<-EOF; }
$1
END
$1
END
		    foo
	EOF
__IN__
{
   0<<END 4<<\END 0<<-EOF
${1}
END
$1
END
    foo
EOF
}
__OUT__

test_single 'here-document, hyphen-prefixed operand'
{
cat fifo << -FOO <<--BA\R
-FOO
-BAR
}
__IN__
{ cat fifo 0<< -FOO 0<<- -BA\R; }
__OUT__

test_multi 'process redirection'
{ <(
    :&
    echo foo
    ) >(
    :&
    echo bar
    ); }
__IN__
{
   0<(:&
      echo foo) 1>(:&
      echo bar)
}
__OUT__

test_multi 'complex simple command'
{ >/dev/null v=0 3</dev/null echo 2>/dev/null : <(); }
__IN__
{
   v=0 echo : 1>/dev/null 3</dev/null 2>/dev/null 0<()
}
__OUT__

test_multi 'word w/ expansions'
{ echo ~/"$1"/$2/${foo}/$((1 + $3))/$(echo 5)/`echo 6`; }
__IN__
{
   echo ~/"${1}"/${2}/${foo}/$((1 + ${3}))/$(echo 5)/$(echo 6)
}
__OUT__

test_multi 'parameter expansion, #-prefixed'
{ echo "${#3}"; }
__IN__
{
   echo "${#3}"
}
__OUT__

test_multi 'parameter expansion, nested'
{ echo "${{#3}-unset}"; }
__IN__
{
   echo "${${#3}-unset}"
}
__OUT__

test_multi 'parameter expansion, indexed'
{ echo "${foo[1]}${bar[2,$((1+2))]}"; }
__IN__
{
   echo "${foo[1]}${bar[2,$((1+2))]}"
}
__OUT__

test_multi 'parameter expansion, w/ basic modifier'
{ echo "${foo:+1}${bar-2}${baz:=3}${xxx?4}"; }
__IN__
{
   echo "${foo:+1}${bar-2}${baz:=3}${xxx?4}"
}
__OUT__

test_multi 'parameter expansion, w/ matching'
{ echo "${foo#x}${bar##y}${baz%z}${xxx%%0}"; }
__IN__
{
   echo "${foo#x}${bar##y}${baz%z}${xxx%%0}"
}
__OUT__

test_multi 'parameter expansion, w/ substitution'
{ echo "${a/x}${b//y}${c/#z/Z}${d/%0/0}${e:/1/2}"; }
__IN__
{
   echo "${a/x/}${b//y/}${c/#z/Z}${d/%0/0}${e:/1/2}"
}
__OUT__

test_multi 'command substitution starting with subshell'
{ echo "$((foo);)"; }
__IN__
{
   echo "$( (foo))"
}
__OUT__

test_multi 'backquoted command substitution'
{ echo "`echo \`echo foo\``"; }
__IN__
{
   echo "$(echo $(echo foo))"
}
__OUT__

test_multi 'complex indentation of compound commands'
{
    (
    if :; foo; then
        for i in 1 2 3; do
            while :; foo& do
                case i in
                    (i)
                        [[ foo ]]
                        f() {
                            cat - /dev/null <<-END
			$(here; document)
			END
                            echo ${foo-$((1 + $(bar; baz)))}
                            cat <(foo; bar <<-END
			END
                            ) >/dev/null
                        }
                esac
            done
            until :; bar& do
                :
            done
        done
    elif :; bar& then
        :
    else
        baz
    fi
    )
}
__IN__
{
   (if :
         foo
      then
         for i in 1 2 3
         do
            while :
               foo&
            do
               case i in
                  (i)
                     [[ foo ]]
                     f()
                     {
                        cat - /dev/null 0<<-END
$(here
   document)
END
                        echo ${foo-$((1 + $(bar
                           baz)))}
                        cat 0<(foo
                           bar 0<<-END
END
                           ) 1>/dev/null
                     }
                     ;;
               esac
            done
            until :
               bar&
            do
               :
            done
         done
      elif :
         bar&
      then
         :
      else
         baz
      fi)
}
__OUT__

test_single 'nested here-documents and command substitutions, single line'
{
    cat fifo <<END1 <<END2
 $(<<EOF11; <<EOF12
foo
EOF11
bar
EOF12
)
END1
 $(<<EOF21; <<EOF22
foo
EOF21
bar
EOF22
)
END2
}
__IN__
{ cat fifo 0<<END1 0<<END2; }
__OUT__

test_multi 'nested here-documents and command substitutions, multi-line'
{
    <<END1 <<END2
 $(<<EOF11; <<EOF12
foo
EOF11
bar
EOF12
)
END1
 $(<<EOF21; <<EOF22
foo
EOF21
bar
EOF22
)
END2
}
__IN__
{
   0<<END1 0<<END2
 $(0<<EOF11
foo
EOF11
   0<<EOF12
bar
EOF12
   )
END1
 $(0<<EOF21
foo
EOF21
   0<<EOF22
bar
EOF22
   )
END2
}
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
