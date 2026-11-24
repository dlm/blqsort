package blqsort

import (
	"cmp"
	"math/rand"
	"slices"
	"testing"
)

type gen struct {
	name string
	f    func(rng *rand.Rand, n int) []float64
}

var gens = []gen{
	{"random", func(rng *rand.Rand, n int) []float64 {
		s := make([]float64, n)
		for i := range s {
			s[i] = rng.Float64()
		}
		return s
	}},
	{"fewunique", func(rng *rand.Rand, n int) []float64 {
		s := make([]float64, n)
		for i := range s {
			s[i] = float64(rng.Intn(10))
		}
		return s
	}},
	{"dups4", func(rng *rand.Rand, n int) []float64 {
		s := make([]float64, n)
		for i := range s {
			s[i] = float64(rng.Intn(n/4 + 1))
		}
		return s
	}},
	{"allequal", func(rng *rand.Rand, n int) []float64 { return make([]float64, n) }},
	{"sorted", func(rng *rand.Rand, n int) []float64 {
		s := make([]float64, n)
		for i := range s {
			s[i] = float64(i)
		}
		return s
	}},
	{"reversed", func(rng *rand.Rand, n int) []float64 {
		s := make([]float64, n)
		for i := range s {
			s[i] = float64(n - i)
		}
		return s
	}},
	{"sawtooth", func(rng *rand.Rand, n int) []float64 {
		s := make([]float64, n)
		for i := range s {
			s[i] = float64(i % 1000)
		}
		return s
	}},
	{"mostlysorted", func(rng *rand.Rand, n int) []float64 {
		s := make([]float64, n)
		for i := range s {
			s[i] = float64(i)
		}
		for range n / 100 {
			i, j := rng.Intn(n), rng.Intn(n)
			s[i], s[j] = s[j], s[i]
		}
		return s
	}},
}

func testSizes(rng *rand.Rand) []int {
	sizes := []int{0, 1, 2, 3, 17, 18, 19, 100, 1024, 1025, 1026, 2000, 5000, 100000, 1 << 20}
	for range 40 {
		sizes = append(sizes, rng.Intn(20000))
	}
	return sizes
}

// cmpF64 is a plain float comparator (no NaN handling).
func cmpF64(a, b float64) int { return b2i(a > b) - b2i(a < b) }

func TestSort(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, g := range gens {
		for _, n := range testSizes(rng) {
			data := g.f(rng, n)
			want := slices.Clone(data)
			slices.Sort(want)
			for name, sort := range map[string]func([]float64){
				"Sort":      Sort[float64],
				"SortFunc":  func(x []float64) { SortFunc(x, cmpF64) },
				"SortLess":  func(x []float64) { SortLess(x, func(a, b float64) bool { return a < b }) },
				"SortByKey": func(x []float64) { SortByKey(x, func(v float64) float64 { return v }) },
			} {
				got := slices.Clone(data)
				sort(got)
				if !slices.Equal(got, want) {
					t.Fatalf("%s %s n=%d: not sorted correctly", name, g.name, n)
				}
			}
		}
	}
}

func TestSortInts(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for _, n := range []int{10, 1000, 100000, 1 << 20} {
		data := make([]int, n)
		for i := range data {
			data[i] = rng.Int()
		}
		want := slices.Clone(data)
		slices.Sort(want)
		Sort(data)
		if !slices.Equal(data, want) {
			t.Fatalf("ints n=%d: not sorted correctly", n)
		}
	}
}

func TestSortStrings(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	for _, n := range []int{0, 1, 19, 1025, 5000, 100000} {
		d := make([]string, n)
		b := make([]byte, 4)
		for i := range d {
			l := 1 + rng.Intn(len(b))
			for j := range l {
				b[j] = byte('a' + rng.Intn(26))
			}
			d[i] = string(b[:l])
		}
		want := slices.Clone(d)
		slices.Sort(want)
		Sort(d)
		if !slices.Equal(d, want) {
			t.Fatalf("n=%d: strings not sorted", n)
		}
	}
}

type rec struct {
	id, value int32
}

type rec64 struct {
	id  int64
	pad [7]int64
}

// TestSortRecords sorts records by key, where elements with equal keys are
// distinguishable, and checks order and that no element was lost or
// duplicated.
func TestSortRecords(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	byID := func(a, b rec) int { return cmp.Compare(a.id, b.id) }
	full := func(a, b rec) int { return cmp.Or(byID(a, b), cmp.Compare(a.value, b.value)) }
	byID64 := func(a, b rec64) int { return cmp.Compare(a.id, b.id) }
	full64 := func(a, b rec64) int { return cmp.Or(byID64(a, b), cmp.Compare(a.pad[0], b.pad[0])) }
	for _, n := range []int{0, 1, 2, 18, 19, 1025, 5000, 100000, 1 << 20} {
		for _, keys := range []int{1, n/4 + 1, 1 << 30} {
			data := make([]rec, n)
			data64 := make([]rec64, n)
			for i := range data {
				data[i] = rec{int32(rng.Intn(keys)), int32(i)}
				data64[i] = rec64{int64(rng.Intn(keys)), [7]int64{int64(i)}}
			}
			checkRecords(t, "SortFunc", data, byID, full, func(x []rec) { SortFunc(x, byID) })
			checkRecords(t, "SortLess", data, byID, full, func(x []rec) { SortLess(x, func(a, b rec) bool { return a.id < b.id }) })
			checkRecords(t, "SortByKey", data, byID, full, func(x []rec) { SortByKey(x, func(r rec) int32 { return r.id }) })
			checkRecords(t, "SortByKey64", data64, byID64, full64, func(x []rec64) { SortByKey(x, func(r rec64) int64 { return r.id }) })
		}
	}
}

func checkRecords[T any](t *testing.T, name string, data []T, byKey, full func(a, b T) int, sort func([]T)) {
	t.Helper()
	got := slices.Clone(data)
	sort(got)
	if !slices.IsSortedFunc(got, byKey) {
		t.Fatalf("%s n=%d: not sorted", name, len(data))
	}
	want := slices.Clone(data)
	slices.SortFunc(want, full)
	slices.SortFunc(got, full)
	if !slices.EqualFunc(got, want, func(a, b T) bool { return full(a, b) == 0 }) {
		t.Fatalf("%s n=%d: elements lost or duplicated", name, len(data))
	}
}

func TestNoAllocs(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, n := range []int{5, 100, 2000, 100000} {
		master := make([]float64, n)
		for i := range master {
			master[i] = rng.Float64()
		}
		data := make([]float64, n)
		recs := make([]rec, n)
		for name, f := range map[string]func(){
			"Sort":     func() { copy(data, master); Sort(data) },
			"SortFunc": func() { copy(data, master); SortFunc(data, cmpF64) },
			"SortLess": func() { copy(data, master); SortLess(data, func(a, b float64) bool { return a < b }) },
			"SortByKey": func() {
				for i := range recs {
					recs[i].id = int32(master[i] * 1e9)
				}
				SortByKey(recs, func(r rec) int32 { return r.id })
			},
		} {
			if a := testing.AllocsPerRun(5, f); a != 0 {
				t.Errorf("%s n=%d: %v allocs", name, n, a)
			}
		}
	}
}

// BenchmarkSort is a quick sanity check against slices.Sort on random data;
// github.com/dlm/blqsort-experiments has the full comparisons.
func BenchmarkSort(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	master := make([]int64, 1_000_000)
	for i := range master {
		master[i] = rng.Int63()
	}
	data := make([]int64, len(master))
	for _, c := range []struct {
		name string
		sort func([]int64)
	}{{"slices.Sort", slices.Sort[[]int64]}, {"blqsort.Sort", Sort[int64]}} {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				copy(data, master)
				b.StartTimer()
				c.sort(data)
			}
		})
	}
}
