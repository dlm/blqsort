package blqsort

import (
	"math/rand"
	"slices"
	"testing"
)

func TestBlqSortDuplicates(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for _, n := range []int{2000, 5000, 20000, 100000, 1 << 20} {
		for _, k := range []int{2, 4, 16, 100} {
			data := make([]int, n)
			for i := range data {
				data[i] = rng.Intn(n/k + 1)
			}
			Sort(data)
			if !slices.IsSorted(data) {
				i := 1
				for data[i-1] <= data[i] {
					i++
				}
				t.Errorf("n=%d dups~%d: unsorted at %d: %v", n, k, i, data[max(0, i-3):i+3])
			}
		}
	}
}

func TestSortingNetworks(t *testing.T) {
	for n := 2; n <= 18; n++ {
		v := make([]int, n)
		for m := 0; m < 1<<n; m++ {
			for i := range v {
				v[i] = m >> i & 1
			}
			sortingNetwork(v)
			if !slices.IsSorted(v) {
				t.Errorf("network %d fails on %b -> %v", n, m, v)
				break
			}
		}
	}
}

func TestSmallSort(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	bp := make([]int, smallPart+1)
	for iter := 0; iter < 20000; iter++ {
		n := 2 + rng.Intn(1024)
		k := 1 + rng.Intn(n)
		data := make([]int, n)
		for i := range data {
			data[i] = rng.Intn(k)
		}
		orig := slices.Clone(data)
		smallSort(data, 0, n-1, bp)
		if !slices.IsSorted(data) {
			t.Fatalf("smallSort n=%d k=%d failed: %v", n, k, orig)
		}
	}
}

func TestSortingNetworksFunc(t *testing.T) {
	for n := 2; n <= 18; n++ {
		v := make([]int, n)
		for m := 0; m < 1<<n; m++ {
			for i := range v {
				v[i] = m >> i & 1
			}
			sortingNetworkFunc(v, func(a, b int) int { return a - b })
			if !slices.IsSorted(v) {
				t.Errorf("network %d fails on %b -> %v", n, m, v)
				break
			}
		}
	}
}

func permutations(n int) [][]int {
	if n == 0 {
		return [][]int{{}}
	}
	var out [][]int
	for _, p := range permutations(n - 1) {
		for i := 0; i <= len(p); i++ {
			q := append(append(append([]int{}, p[:i]...), n-1), p[i:]...)
			out = append(out, q)
		}
	}
	return out
}

func TestMedianNetworks(t *testing.T) {
	for _, p := range permutations(5) {
		if _, _, c, _, _ := med5(p[0], p[1], p[2], p[3], p[4]); c != 2 {
			t.Fatalf("med5%v = %d", p, c)
		}
	}
	for _, p := range permutations(7) {
		if _, _, _, d, _, _, _ := med7(p[0], p[1], p[2], p[3], p[4], p[5], p[6]); d != 3 {
			t.Fatalf("med7%v = %d", p, d)
		}
	}
}

// Inputs that start descending, at sizes around the run-limit boundaries
// (runLimit is 1 for n < 32): the reversed-input shortcut must only fire when
// the scan was complete.
func TestScanOrderEdges(t *testing.T) {
	for n := 19; n <= 1100; n++ {
		shapes := map[string][]int{}
		d := make([]int, n)
		for i := range d {
			d[i] = i
		}
		d[0], d[1] = d[1], d[0]
		shapes["desc-then-asc"] = d
		r := make([]int, n)
		for i := range r {
			r[i] = n - i
		}
		shapes["reversed"] = r
		r2 := slices.Clone(r)
		r2[n-1] = n + 1
		shapes["reversed-but-last"] = r2
		v := make([]int, n)
		for i := range v {
			v[i] = abs(i - n/2)
		}
		shapes["valley"] = v
		for name, s := range shapes {
			got := slices.Clone(s)
			Sort(got)
			if !slices.IsSorted(got) {
				t.Fatalf("%s n=%d: not sorted", name, n)
			}
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
