package util

// InsertionSortThreshold is the slice size after which we should consider
// using the stdlib sort method instead of the InsertionSort implemented below.
const InsertionSortThreshold = 40

// InsertionSort sorts in-place the given elements, not doing any allocation.
// It is very efficient for on slices but if memory allocation is not an issue,
// consider using the stdlib `sort.Sort` method on slices having a size > InsertionSortThreshold.
// See `pkg/util/sort_benchmarks_note.md` for more details.
func InsertionSort(elements []string) {
	for i := 1; i < len(elements); i++ {
		temp := elements[i]
		j := i
		for j > 0 && temp <= elements[j-1] {
			elements[j] = elements[j-1]
			j--
		}
		elements[j] = temp
	}
}
