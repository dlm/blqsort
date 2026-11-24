// Package blqsort is a branchless quicksort for Go, a port of blqsort by
// Christof Kaser (https://github.com/chkas/blqsort, MIT).
//
// It is fastest when comparisons are unpredictable, i.e. on random data:
// about 2.5-3x faster than slices.Sort for large random inputs on amd64
// (Intel), 3-3.7x on arm64 (Apple M1 Pro), and ahead from 100 elements up
// (1.4x / 2.7x). Benchmarks that sort the same small input repeatedly let
// the branch predictor learn it, which hides most of that gain below about
// 10K elements.
// On data with regular structure (periodic or partly sorted) the branch
// predictor makes slices.Sort's branchy partitioning cheap, and slices.Sort
// can be faster. None of the sorts are stable.
//
// Sort is for cmp.Ordered types. For other types prefer SortByKey or
// SortLess over SortFunc: a three-way comparator built with cmp.Compare
// compiles to unpredictable branches, which defeats the branchless design.
//
// Implementation notes: pointer arithmetic from the C version is translated
// to indices into a single slice. The scratch buffer is one stack array per
// call, passed down, because Go zero-initializes stack arrays on every
// function call. Changes from the C version are described where they occur.
package blqsort

//go:generate go run ./gen

import (
	"cmp"
	"math/bits"
	"slices"
)

const (
	smallPart = 1024
	swsz      = 384
	unroll    = 16
)

// s2 is the branchless compare-exchange used by the sorting networks.
func s2[T cmp.Ordered](x, y T) (T, T) {
	if y < x {
		return y, x
	}
	return x, y
}

func b2i(b bool) int {
	var i int
	if b {
		i = 1
	}
	return i
}

// pivotJitter picks the 0..7 offset of the pivot samples. The C version uses
// a global PRNG, which would be a data race in Go; a hash of the partition
// bounds is deterministic (like the stdlib pdqsort) and the heapsort fallback
// still bounds the worst case.
func pivotJitter(left, right int) int {
	h := uint64(left)*0x9E3779B97F4A7C15 ^ uint64(right)
	h ^= h >> 29
	return int(h * 0xBF58476D1CE4E5B9 >> 61)
}

// depthLimit is the number of lopsided splits sortr tolerates before
// falling back to heapsort.
func depthLimit(n int) int {
	return bits.Len(uint(n))
}

// runLimit is the number of monotone runs (as counted by scanOrder) up to
// which pdqsort's pattern detection beats blqsort; measured on amd64.
func runLimit(n int) int {
	if n <= 1024 {
		return n / 16
	}
	return 16
}

// mayHaveNaN reports whether T can hold NaNs. Unknown (named) types answer
// true, which is always correct, just slower.
func mayHaveNaN[T cmp.Ordered]() bool {
	switch any(*new(T)).(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr, string:
		return false
	}
	return true
}

// moveNaNsFirst moves NaNs to the front, where slices.Sort orders them, and
// returns the rest.
func moveNaNsFirst[T cmp.Ordered](data []T) []T {
	if !mayHaveNaN[T]() {
		return data
	}
	nan := 0
	for i := range data {
		if isNaN(data[i]) {
			data[nan], data[i] = data[i], data[nan]
			nan++
		}
	}
	return data[nan:]
}

// isNaN reports whether x is a NaN without requiring a float type.
func isNaN[T cmp.Ordered](x T) bool {
	return x != x
}

func heapSort[T cmp.Ordered](a []T) {
	n := len(a)
	if n < 2 {
		return
	}
	for i := n / 2; ; {
		var k T
		if i > 0 {
			i--
			k = a[i]
		} else {
			n--
			if n == 0 {
				return
			}
			k = a[n]
			a[n] = a[0]
		}
		j := i
		for j*2+1 < n {
			child := j*2 + 1
			if child+1 < n && a[child] < a[child+1] {
				child++
			}
			if !(k < a[child]) {
				break
			}
			a[j] = a[child]
			j = child
		}
		a[j] = k
	}
}

func partitionSmall[T cmp.Ordered](a []T, left, right int, buf []T) int {
	outerleft := left
	pivp := left + (right-left)/2
	piv := a[pivp]

	// Median of 7 samples spread evenly over the range. The C version samples
	// both ends and the middle (and then skips the end samples), which picks
	// extreme pivots on inputs with positional structure.
	step := (right - left) / 8
	p0, p1, p2 := pivp-3*step, pivp-2*step, pivp-step
	p4, p5, p6 := pivp+step, pivp+2*step, pivp+3*step
	a[p0], a[p1], a[p2], piv, a[p4], a[p5], a[p6] = med7(a[p0], a[p1], a[p2], piv, a[p4], a[p5], a[p6])
	left++

	a[pivp] = a[outerleft]

	sw := 0
	lwr := left
	for right-left >= unroll {
		for range unroll {
			x := a[left]
			h := b2i(x < piv)
			a[lwr] = x
			buf[sw] = x
			left++
			lwr += h
			sw += 1 - h
		}
	}
	for left <= right {
		x := a[left]
		h := b2i(x < piv)
		a[lwr] = x
		buf[sw] = x
		left++
		lwr += h
		sw += 1 - h
	}
	copy(a[lwr:], buf[:sw])
	lwr--
	a[outerleft] = a[lwr]
	a[lwr] = piv
	return lwr
}

func partition[T cmp.Ordered](a []T, left, right int, buf []T) int {
	outerleft := left
	pivp := left + (right-left)/2
	piv := a[pivp]

	// Pivot: median of 7 medians of 5, as in the C version, but the 35 samples
	// are spread evenly over the range (sample 17 is pivp) instead of taken
	// in consecutive runs at both ends and the middle, which picked extreme
	// pivots on inputs with positional structure.
	j := pivotJitter(left, right)
	step := (right - left) / 36
	at := func(k int) int {
		return pivp + (k-17)*step + j
	}
	m5 := func(k int) {
		p0, p1, p2, p3, p4 := at(k), at(k+1), at(k+2), at(k+3), at(k+4)
		a[p0], a[p1], a[p2], a[p3], a[p4] = med5(a[p0], a[p1], a[p2], a[p3], a[p4])
	}
	m5(0)
	m5(5)
	m5(10)
	p15, p16, p18, p19 := at(15), at(16), at(18), at(19)
	a[p15], a[p16], piv, a[p18], a[p19] = med5(a[p15], a[p16], piv, a[p18], a[p19])
	m5(20)
	m5(25)
	m5(30)
	p2, p7, p12, p22, p27, p32 := at(2), at(7), at(12), at(22), at(27), at(32)
	a[p2], a[p7], a[p12], piv, a[p22], a[p27], a[p32] = med7(a[p2], a[p7], a[p12], piv, a[p22], a[p27], a[p32])

	left++
	a[pivp] = a[outerleft]

	rwr, sw := right, 0
	lwr := left

	for unroll < swsz-sw && left < right-unroll {
		avail := min(right-left, swsz-sw)
		endp := right - avail
		for right > endp+unroll {
			for range unroll {
				x := a[right]
				h := b2i(x < piv)
				a[rwr] = x
				buf[sw] = x
				right--
				rwr -= 1 - h
				sw += h
			}
		}
	}

	for right-left >= unroll && (rwr-right > unroll || left-lwr > unroll) {
		for rwr-right > unroll && right-left >= unroll {
			for range unroll {
				x := a[left]
				h := b2i(x < piv)
				a[lwr] = x
				a[rwr] = x
				left++
				lwr += h
				rwr -= 1 - h
			}
		}
		for left-lwr > unroll && right-left >= unroll {
			for range unroll {
				x := a[right]
				h := b2i(x < piv)
				a[rwr] = x
				a[lwr] = x
				right--
				rwr -= 1 - h
				lwr += h
			}
		}
	}
	for (lwr < left || rwr > right) && left <= right {
		for rwr > right && left <= right {
			x := a[left]
			left++
			if x < piv {
				a[lwr] = x
				lwr++
			} else {
				a[rwr] = x
				rwr--
			}
		}
		for lwr < left && left <= right {
			x := a[right]
			right--
			if x < piv {
				a[lwr] = x
				lwr++
			} else {
				a[rwr] = x
				rwr--
			}
		}
	}
	for left <= right {
		if a[left] < piv {
			left++
			lwr++
		} else {
			a[left], a[right] = a[right], a[left]
			right--
			rwr--
		}
	}
	copy(a[lwr:], buf[:sw])
	a[outerleft] = a[rwr]
	a[rwr] = piv
	return rwr
}

// smallSort differs from the C version, which has no duplicate handling below
// SMALLPART and goes quadratic within a part when most elements equal the
// pivot. Like sortr, a tiny left side collects the pivot's duplicates, splits
// that make no real progress count against an introsort-style limit, and
// recursion goes into the smaller side.
func smallSort[T cmp.Ordered](a []T, left, right int, buf []T) {
	limit := depthLimit(right - left + 1)
	for right-left > 17 {
		n := right - left
		// Part already sorted (pdqsort detects this too); costs about 16
		// compares otherwise.
		if runs, _ := scanOrder(a[left:right+1], 1); runs == 1 {
			return
		}
		mid := partitionSmall(a, left, right, buf)
		if mid-left < n/16 {
			smallSort(a, left, mid-1, buf)
			piv := a[mid]
			mid++
			// collect duplicates
			for p := mid; p <= right; p++ {
				if !(piv < a[p]) {
					a[mid], a[p] = a[p], a[mid]
					mid++
				}
			}
			if mid-left < n/16 {
				if limit--; limit < 0 {
					heapSort(a[mid : right+1])
					return
				}
			}
			left = mid
			continue
		}
		if right-mid < n/16 {
			if limit--; limit < 0 {
				heapSort(a[left : right+1])
				return
			}
		}
		if mid-left < right-mid {
			smallSort(a, left, mid-1, buf)
			left = mid + 1
		} else {
			smallSort(a, mid+1, right, buf)
			right = mid - 1
		}
	}
	sortingNetwork(a[left : right+1])
}

// sortr differs from the C version, which only detects a tiny left side and
// then gives one second chance before heapsorting the whole part. Here a tiny
// left side collects the pivot's duplicates (as in C), and any split that
// makes no real progress, on either side, counts against an introsort-style
// limit before falling back to heapsort.
func sortr[T cmp.Ordered](a []T, left, right int, buf []T, limit int) {
	for right-left > smallPart {
		n := right - left
		// Part already sorted (pdqsort detects this too); costs about 16
		// compares otherwise.
		if runs, _ := scanOrder(a[left:right+1], 1); runs == 1 {
			return
		}
		mid := partition(a, left, right, buf)
		if mid-left < n/16 {
			if mid > left {
				sortr(a, left, mid-1, buf, limit)
			}
			piv := a[mid]
			mid++
			// collect duplicates
			for p := mid; p <= right; p++ {
				if !(piv < a[p]) {
					a[mid], a[p] = a[p], a[mid]
					mid++
				}
			}
			if mid-left < n/16 {
				if limit--; limit < 0 {
					heapSort(a[mid : right+1])
					return
				}
			}
			left = mid
			continue
		}
		if right-mid < n/16 {
			if limit--; limit < 0 {
				heapSort(a[left : right+1])
				return
			}
		}
		if mid-left < right-mid {
			sortr(a, left, mid-1, buf, limit)
			left = mid + 1
		} else {
			sortr(a, mid+1, right, buf, limit)
			right = mid - 1
		}
	}
	smallSort(a, left, right, buf)
}

// scanOrder counts the alternating non-decreasing / non-increasing runs in
// data, stopping once there are more than k. Each run is consumed by a tight
// loop whose branch is predictable inside a run (equal elements continue the
// run, so duplicates cost nothing), as in pdqsort's sortedness check; on
// unstructured data it gives up after about k comparisons. A drop between two
// ascending runs counts as a short descending run, so this over-counts by up
// to 2x; runLimit is calibrated for that. ascEnd is where the first
// non-decreasing run ends: runs == 1 means sorted, and runs == 2 &&
// ascEnd == 1 means non-increasing if k >= 2 (so the second run was scanned
// to the end).
func scanOrder[T cmp.Ordered](data []T, k int) (runs, ascEnd int) {
	n := len(data)
	i := 1
	for i < n && !(data[i] < data[i-1]) {
		i++
	}
	runs, ascEnd = 1, i
	for i < n {
		if runs++; runs > k {
			break
		}
		if runs%2 == 0 {
			for i < n && !(data[i-1] < data[i]) {
				i++
			}
		} else {
			for i < n && !(data[i] < data[i-1]) {
				i++
			}
		}
	}
	return runs, ascEnd
}

// blqSort is the entry point after NaN handling. The comparator variants
// are derived from everything above this point plus this function.
func blqSort[T cmp.Ordered](data []T) {
	n := len(data)
	if n < 2 {
		return
	}
	if n <= 18 {
		sortingNetwork(data)
		return
	}
	// One pass detects sorted, reversed and few-runs input; on unstructured
	// data it stops after a few dozen comparisons.
	lim := max(runLimit(n), 2)
	switch runs, ascEnd := scanOrder(data, lim); {
	case runs == 1:
		return
	case runs == 2 && ascEnd == 1: // complete scan, since lim >= 2
		// Non-increasing; reversing it sorts it.
		for i := 0; i < n/2; i++ {
			data[i], data[n-1-i] = data[n-1-i], data[i]
		}
		return
	case runs <= runLimit(n):
		// A few long runs: pdqsort is near-linear there, blqsort is not.
		slices.Sort(data)
		return
	}
	var buf [smallPart + 1]T
	sortr(data, 0, n-1, buf[:], depthLimit(n))
}

// Sort sorts data in ascending order. Like slices.Sort, NaNs are ordered
// before other values.
func Sort[T cmp.Ordered](data []T) {
	blqSort(moveNaNsFirst(data))
}
