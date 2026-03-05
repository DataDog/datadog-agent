# history2-y.tst: yash-specific test of history, part 2

if ! testee -c 'command -bv fc history' >/dev/null; then
    skip="true"
fi

cat >rcfile1 <<\__END__
PS1= PS2= HISTFILE=$PWD/$histfile HISTSIZE=$histsize
unset HISTRMDUP
__END__

(
export histfile=histfile$LINENO histsize=30

test_oE 'many history entries'
(
i=32767  # < HISTORY_MIN_MAX_NUMBER
while [ $i -gt 0 ]; do
    echo : $(( i-- ))
done
echo history
) |
"$TESTEE" -i +m --rcfile="rcfile1"
__IN__
32739	: 29
32740	: 28
32741	: 27
32742	: 26
32743	: 25
32744	: 24
32745	: 23
32746	: 22
32747	: 21
32748	: 20
32749	: 19
32750	: 18
32751	: 17
32752	: 16
32753	: 15
32754	: 14
32755	: 13
32756	: 12
32757	: 11
32758	: 10
32759	: 9
32760	: 8
32761	: 7
32762	: 6
32763	: 5
32764	: 4
32765	: 3
32766	: 2
32767	: 1
32768	history
__OUT__

)

# vim: set ft=sh ts=8 sts=4 sw=4 et:
