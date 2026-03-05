# case-p.tst: test of case command for any POSIX-compliant shell

posix="true"

test_oE 'case word is subject to tilde expansion'
HOME=/home
case ~/foo in
    /home/foo) echo matched;;
esac
__IN__
matched
__OUT__

test_oE 'case word is subject to parameter expansion'
HOME=/home
case $HOME/foo in
    /home/foo) echo matched;;
esac
__IN__
matched
__OUT__

test_oE 'case word is subject to command substitution'
case $(echo foo)`echo bar` in
    foobar) echo matched;;
esac
__IN__
matched
__OUT__

test_oE 'case word is subject to arithmetic expansion'
case $((1+2)) in
    3) echo matched;;
esac
__IN__
matched
__OUT__

test_oE 'case word is subject to quote removal'
w='"1"'"'2'"\3
case '"1"'"'2'"\3 in
    $w) echo matched;;
esac
__IN__
matched
__OUT__

test_oE 'quotations arising from expansion in case word'
# XCU 2.6.7 says:
#   The quote characters that were present in the original word shall be
#   removed unless they have themselves been quoted.
# That means backslashes in the word are not special here (because they are
# arising from expansion, not in the original word).
bs='\a\z'
case  $bs  in '\a\z') echo bs1; esac
case "$bs" in '\a\z') echo bs2; esac
__IN__
bs1
bs2
__OUT__

test_oE 'case pattern is subject to tilde expansion'
HOME=/home
case /home/foo in
    ~/foo) echo matched;;
esac
__IN__
matched
__OUT__

test_oE 'case pattern is subject to parameter expansion'
HOME=/home
case /home/foo in
    $HOME/foo) echo matched;;
esac
__IN__
matched
__OUT__

test_oE 'case pattern is subject to command substitution'
case foobar in
    $(echo foo)`echo bar`) echo matched;;
esac
__IN__
matched
__OUT__

test_oE 'case pattern is subject to arithmetic expansion'
case 3 in
    $((1+2))) echo matched;;
esac
__IN__
matched
__OUT__

test_oE 'case pattern is subject to quote removal'
w='"1"'"'2'"\3
case $w in
    '"1"'"'2'"\3) echo matched;;
esac
__IN__
matched
__OUT__

test_oE 'backslashes arising from expansion in case pattern'
# XCU 2.9.4 implies unquoted backslashes are special in the pattern.
bs='\a\z'
case 'az'   in  $bs ) echo bs1; esac
case '\a\z' in "$bs") echo bs2; esac
__IN__
bs1
bs2
__OUT__

test_oE 'pattern matching and quotes (*)'
case '*ab' in
    \*\*\*) echo not reached;;
    '***') echo not reached;;
     "***") echo not reached;;
    \**) echo matched;;
esac
__IN__
matched
__OUT__

test_oE 'pattern matching and quotes (?)'
case '?a' in
    \?\?) echo not reached;;
    '??') echo not reached;;
    "??") echo not reached;;
    \??) echo matched;;
esac
__IN__
matched
__OUT__

test_oE 'pattern matching and quotes ([])'
case '[a' in
    \[\[abc]) echo not reached;;
    '[['abc]) echo not reached;;
    "[["abc]) echo not reached;;
    \[[abc]) echo matched;;
esac
__IN__
matched
__OUT__

test_oE '* and ? match / and .'
case //-/-.-. in
    *-?-*-?) echo matched;;
esac
__IN__
matched
__OUT__

test_oe 'patterns are not expanded after first match'
case 1 in
    $(echo expanded 0 >&2; echo 0)) echo matched 0;;
    $(echo expanded 1 >&2; echo 1)) echo matched 1;;
    $(echo expanded 2 >&2; echo 2)) echo matched 2;;
esac
__IN__
matched 1
__OUT__
expanded 0
expanded 1
__ERR__

test_oE 'multiple patterns for single command list'
case 1 in
    a|b|c) echo not reached;;
    0   | 1     | 2 ) echo matched;;
esac
__IN__
matched
__OUT__

test_OE -e 0 'exit status of case command (unmatched, empty)'
false
case $(false) in
esac
__IN__

test_OE -e 0 'exit status of case command (unmatched, non-empty)'
case $(false) in
    1) true; (exit 11);;
    2) true; (exit 17);;
    3) true; (exit 19);;
esac
__IN__

# The behavior is POSIXly-unspecified for this case. See case-y.tst.
#test_OE -e 0 'exit status of case command (matched, empty)'

test_OE -e 17 'exit status of case command (matched, non-empty)'
case $(echo 2; exit 2) in
    1) true; (exit 11);;
    2) true; (exit 17);;
    3) true; (exit 19);;
esac
__IN__

test_oE -e 42 'executing item after ;&'
case 1 in
    0) echo not reached 0;;
    1) echo matched 1;&
    2) echo matched 2; (exit 42);&
esac
__IN__
matched 1
matched 2
__OUT__

test_oE 'exit status after empty ;& in case command'
(exit 1)
case i in
    i) ;&
    j) echo $?
esac
__IN__
1
__OUT__

test_oE 'patterns can be preceded by ('
case a in
    (a) echo matched 1;;
    (b) echo not reached 1 b;;
    (c) echo not reached 1 c;;
esac
case a in
     a) echo matched 2;;
    (b) echo not reached 2 b;;
     c) echo not reached 2 c;;
    (d) echo not reached 2 d;;
esac
__IN__
matched 1
matched 2
__OUT__

test_oE 'linebreak after word'
case foo

    in foo)echo matched;;esac
__IN__
matched
__OUT__

test_oE 'linebreak after in'
case foo in
    
    foo)echo matched;;esac
__IN__
matched
__OUT__

test_oE 'linebreak after )'
case foo in foo)
    
    echo matched;;esac
__IN__
matched
__OUT__

test_oE 'linebreak before ;;'
case foo in foo)echo matched

    ;;esac
__IN__
matched
__OUT__

test_oE '; before ;;'
case foo in foo)echo matched; ;;esac
__IN__
matched
__OUT__

test_oE '& before ;;'
case foo in foo)echo matched&;;esac
wait
__IN__
matched
__OUT__

test_oE 'linebreak after ;;'
case foo in bar)echo not reached;;
    
    foo)echo matched;;esac
__IN__
matched
__OUT__

test_oE 'linebreak before esac'
case foo in foo)echo matched;;

esac
__IN__
matched
__OUT__

test_oE ';; can be omitted before esac'
case 2 in
    0) echo a;;
    1) echo b;;
    2) echo c
esac
case 2 in
    0) echo A;;
    1) echo B;;
    2) echo C; esac
case 2 in
    0) echo A;;
    1) echo B;;
    *)
esac
case 2 in
    0) echo A;;
    1) echo B;;
    *) esac
__IN__
c
C
__OUT__

# $1 = LINENO
# $2 = reserved word
test_reserved_word_as_pattern() {
    testcase "$1" "reserved word $2 as pattern" 5<&- 3<<__IN__ 4<<\__OUT__
case $2 in $2) echo matched;; esac
__IN__
matched
__OUT__
}

test_reserved_word_as_pattern "$LINENO" !
test_reserved_word_as_pattern "$LINENO" {
test_reserved_word_as_pattern "$LINENO" }
test_reserved_word_as_pattern "$LINENO" [[
test_reserved_word_as_pattern "$LINENO" ]]
test_reserved_word_as_pattern "$LINENO" case
test_reserved_word_as_pattern "$LINENO" do
test_reserved_word_as_pattern "$LINENO" done
test_reserved_word_as_pattern "$LINENO" elif
test_reserved_word_as_pattern "$LINENO" else
#test_reserved_word_as_pattern "$LINENO" esac
test_reserved_word_as_pattern "$LINENO" fi
test_reserved_word_as_pattern "$LINENO" for
test_reserved_word_as_pattern "$LINENO" function
test_reserved_word_as_pattern "$LINENO" if
test_reserved_word_as_pattern "$LINENO" in
test_reserved_word_as_pattern "$LINENO" select
test_reserved_word_as_pattern "$LINENO" then
test_reserved_word_as_pattern "$LINENO" until
test_reserved_word_as_pattern "$LINENO" while

test_oE 'esac as first pattern'
case esac in (esac) echo matched;; esac
__IN__
matched
__OUT__

test_oE 'esac as non-first pattern'
case esac in -|esac) echo matched;; esac
__IN__
matched
__OUT__

test_oE 'redirection on case command'
case $(echo foo >&2) in
    $(echo bar >&2)) echo baz >&2;;
esac 2>redir_out
cat redir_out
__IN__
foo
bar
baz
__OUT__

# vim: set ft=sh ts=8 sts=4 sw=4 et:
