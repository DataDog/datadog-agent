# Sort benchmarks - 2020-03-29

I've benchmarked the selection sort (used in Agent 6.18.0 aggregator time sampler) against
 the insertion sort and the Go stdlib sort.

I've benchmarked it with a varying sizes of tag slices o(in my example `bench-sort-15.txt`
is a slice of 15 tags). Each benchmark is runned 20 times and the result is compiled below,
that's why it's showing a ±% variation. Here are the results:

name \ time/op     bench-sort-15.txt  bench-sort-20.txt  bench-sort-25.txt  bench-sort-30.txt
SelectionSort-4           7.9µs ± 1%        11.1µs ± 2%        13.9µs ± 4%        17.1µs ± 1%
InsertionSort-4           7.4µs ± 0%        10.3µs ± 1%        12.9µs ± 2%        15.6µs ± 2%
StdlibSort-4              7.9µs ± 1%        10.6µs ± 1%        13.2µs ± 1%        15.8µs ± 2%
                   bench-sort-35.txt  bench-sort-40.txt  bench-sort-45.txt  bench-sort-50.txt
SelectionSort-4           20.8µs ± 2%        24.2µs ± 1%        27.7µs ± 1%        31.6µs ± 1%
InsertionSort-4           18.7µs ± 0%        21.4µs ± 1%        24.4µs ± 1%        27.4µs ± 1%
StdlibSort-4              18.8µs ± 1%        21.4µs ± 1%        24.1µs ± 1%        26.8µs ± 1%
                   bench-sort-55.txt  bench-sort-60.txt  bench-sort-65.txt  bench-sort-70.txt
SelectionSort-4           35.3µs ± 1%        39.5µs ± 1%        44.5µs ± 1%        48.9µs ± 1%
InsertionSort-4           30.8µs ± 1%        34.0µs ± 2%        38.1µs ± 1%        41.8µs ± 1%
StdlibSort-4              29.4µs ± 1%        32.1µs ± 1%        35.7µs ± 1%        38.5µs ± 1%
                   bench-sort-75.txt  bench-sort-80.txt  bench-sort-85.txt  bench-sort-90.txt
SelectionSort-4           54.1µs ± 2%        58.3µs ± 1%        62.9µs ± 1%        68.0µs ± 1%
InsertionSort-4           45.4µs ± 2%        48.8µs ± 1%        52.2µs ± 1%        56.4µs ± 1%
StdlibSort-4              41.3µs ± 1%        43.8µs ± 1%        46.6µs ± 1%        50.1µs ± 1%
                   bench-sort-95.txt bench-sort-100.txt
SelectionSort-4           72.9µs ± 1%        78.4µs ± 1%
InsertionSort-4           60.1µs ± 1%        64.0µs ± 2%
StdlibSort-4              51.9µs ± 1%        55.0µs ± 1%

You can observe than in all cases, the InsertionSort has better performances than
the SelectionSort, there is no reason to continue to use the SelectionSort.
You can also observe that until 40 tags per slice, the InsertionSort is having slightly
better performances than the Go stdlib sort (which is using insertion sort internally
under some circumstances).

Because the InsertionSort has better performances and because it is not allocating any byte
(the stdlib sort does), we should use it to sort slice having a length < 40.
