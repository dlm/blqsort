// Value-based ports of the stdlib sort/slices tests ($GOROOT/src/sort/sort_test.go,
// $GOROOT/src/slices/sort_test.go). The originals drive sort.Interface by index;
// blqsort moves values, so these check results against slices.Sort instead of
// counting swaps, and count comparisons through SortLess.

package blqsort

import (
	"cmp"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"testing"
)

// Same data as the stdlib tests.
var (
	stdInts     = [...]int{74, 59, 238, -784, 9845, 959, 905, 0, 0, 42, 7586, -5467984, 7586}
	stdFloat64s = [...]float64{74.3, 59.0, math.Inf(1), 238.2, -784.0, 2.3, math.NaN(), math.NaN(), math.Inf(-1), 9845.768, -959.7485, 905, 7.8, 7.8, 74.3, 59.0, math.Inf(1), 238.2, -784.0, 2.3}
	stdStrings  = [...]string{"", "Hello", "foo", "bar", "foo", "f00", "%*&^*&^&", "***"}
)

func nanEqual[T cmp.Ordered](a, b []T) bool {
	return slices.EqualFunc(a, b, func(x, y T) bool { return cmp.Compare(x, y) == 0 })
}

func TestStdlibFixedSlices(t *testing.T) {
	ints := stdInts[:]
	Sort(ints)
	if !slices.IsSorted(ints) {
		t.Errorf("ints: %v", ints)
	}
	f := slices.Clone(stdFloat64s[:])
	want := slices.Clone(f)
	slices.Sort(want)
	Sort(f)
	if !nanEqual(f, want) {
		t.Errorf("float64s with NaN: got %v, want %v", f, want)
	}
	s := stdStrings[:]
	Sort(s)
	if !slices.IsSorted(s) {
		t.Errorf("strings: %v", s)
	}
}

// NaN, ±Inf and ±0 mixes at many sizes, for float64 and float32, compared
// with slices.Sort (NaNs first).
func TestNaNsMatchSlicesSort(t *testing.T) {
	rng := rand.New(rand.NewSource(21))
	specials := []float64{math.NaN(), math.Inf(1), math.Inf(-1), 0, math.Copysign(0, -1)}
	for _, n := range []int{1, 2, 3, 17, 18, 19, 100, 1025, 1026, 5000, 100000} {
		for _, pNaN := range []float64{0, 0.01, 0.5, 1} {
			f := make([]float64, n)
			for i := range f {
				switch {
				case rng.Float64() < pNaN:
					f[i] = math.NaN()
				case rng.Intn(10) == 0:
					f[i] = specials[rng.Intn(len(specials))]
				default:
					f[i] = rng.NormFloat64()
				}
			}
			f32 := make([]float32, n)
			for i := range f {
				f32[i] = float32(f[i])
			}
			want, want32 := slices.Clone(f), slices.Clone(f32)
			slices.Sort(want)
			slices.Sort(want32)
			Sort(f)
			Sort(f32)
			if !nanEqual(f, want) {
				t.Fatalf("float64 n=%d pNaN=%v: mismatch with slices.Sort", n, pNaN)
			}
			if !nanEqual(f32, want32) {
				t.Fatalf("float32 n=%d pNaN=%v: mismatch with slices.Sort", n, pNaN)
			}
		}
	}
}

// SortByKey with NaN keys orders like slices.SortFunc with cmp.Compare on
// the key (NaNs first), also for the elements whose key is not NaN.
func TestSortByKeyNaNs(t *testing.T) {
	type item struct {
		k  float64
		id int
	}
	rng := rand.New(rand.NewSource(22))
	for _, n := range []int{1, 2, 3, 17, 18, 19, 100, 1025, 1026, 5000, 100000} {
		for _, pNaN := range []float64{0, 0.01, 0.05, 0.5, 1} {
			items := make([]item, n)
			for i := range items {
				items[i] = item{rng.NormFloat64(), i}
				if rng.Float64() < pNaN {
					items[i].k = math.NaN()
				}
			}
			want := slices.Clone(items)
			slices.SortFunc(want, func(a, b item) int { return cmp.Compare(a.k, b.k) })
			SortByKey(items, func(x item) float64 { return x.k })
			got := make([]float64, n)
			wantKeys := make([]float64, n)
			for i := range items {
				got[i], wantKeys[i] = items[i].k, want[i].k
			}
			if !nanEqual(got, wantKeys) {
				t.Fatalf("n=%d pNaN=%v: keys differ from slices.SortFunc with cmp.Compare", n, pNaN)
			}
		}
	}
}

func TestStdlibBreakPatterns(t *testing.T) {
	data := make([]int, 30)
	for i := range data {
		data[i] = 10
	}
	data[(len(data)/4)*1] = 0
	data[(len(data)/4)*2] = 1
	data[(len(data)/4)*3] = 2
	Sort(data)
	if !slices.IsSorted(data) {
		t.Fatal(data)
	}
}

func TestStdlibReverseSort(t *testing.T) {
	data := stdInts[:]
	data1 := slices.Clone(data)
	SortFunc(data1, func(a, b int) int { return cmp.Compare(b, a) })
	slices.Sort(data)
	slices.Reverse(data)
	if !slices.Equal(data, data1) {
		t.Fatalf("reverse: got %v, want %v", data1, data)
	}
}

func lg(n int) int {
	i := 0
	for 1<<uint(i) < n {
		i++
	}
	return i
}

// The Bentley-McIlroy "Engineering a Sort Function" matrix from the stdlib
// tests, extended with sizes that reach the large-partition path (> 1025).
func TestBentleyMcIlroy(t *testing.T) {
	sizes := []int{100, 1023, 1024, 1025, 1026, 2049, 10000, 100000}
	if testing.Short() {
		sizes = []int{100, 1025, 2049}
	}
	dists := []string{"sawtooth", "rand", "stagger", "plateau", "shuffle"}
	modes := []string{"copy", "reverse", "reverse1", "reverse2", "sort", "dither"}
	rng := rand.New(rand.NewSource(31))
	worst, worstDesc := 0.0, ""
	pdqWorst, pdqWorstDesc := 0.0, ""
	for _, n := range sizes {
		data, mdata := make([]int, n), make([]int, n)
		for m := 1; m < 2*n; m *= 2 {
			for dist := range dists {
				j, k := 0, 1
				for i := range data {
					switch dist {
					case 0:
						data[i] = i % m
					case 1:
						data[i] = rng.Intn(m)
					case 2:
						data[i] = (i*m + i) % n
					case 3:
						data[i] = min(i, m)
					case 4:
						if rng.Intn(m) != 0 {
							j += 2
							data[i] = j
						} else {
							k += 2
							data[i] = k
						}
					}
				}
				for mode := range modes {
					switch mode {
					case 0:
						copy(mdata, data)
					case 1:
						for i := range n {
							mdata[i] = data[n-i-1]
						}
					case 2:
						for i := 0; i < n/2; i++ {
							mdata[i] = data[n/2-i-1]
						}
						copy(mdata[n/2:], data[n/2:])
					case 3:
						copy(mdata[:n/2], data[:n/2])
						for i := n / 2; i < n; i++ {
							mdata[i] = data[n-(i-n/2)-1]
						}
					case 4:
						copy(mdata, data)
						slices.Sort(mdata)
					case 5:
						for i := range n {
							mdata[i] = data[i] + i%5
						}
					}
					desc := fmt.Sprintf("n=%d m=%d dist=%s mode=%s", n, m, dists[dist], modes[mode])
					want := slices.Clone(mdata)
					slices.Sort(want)

					got := slices.Clone(mdata)
					Sort(got)
					if !slices.Equal(got, want) {
						t.Fatalf("%s: Sort result wrong", desc)
					}

					got = slices.Clone(mdata)
					ncmp := 0
					SortLess(got, func(a, b int) bool { ncmp++; return a < b })
					if !slices.Equal(got, want) {
						t.Fatalf("%s: SortLess result wrong", desc)
					}
					// The stdlib allows 1.2*n*lg(n) swaps; compares are the analogous budget here.
					if r := float64(ncmp) / float64(n*lg(n)); r > worst {
						worst, worstDesc = r, desc
					}
					pcmp := 0
					slices.SortFunc(slices.Clone(mdata), func(a, b int) int { pcmp++; return cmp.Compare(a, b) })
					if r := float64(pcmp) / float64(n*lg(n)); r > pdqWorst {
						pdqWorst, pdqWorstDesc = r, desc
					}
					// Same budget as the stdlib TestAdversary (4 n lg n).
					if ncmp > 4*n*lg(n) {
						t.Fatalf("%s: %d compares (%.2f n lg n), pdqsort %d", desc, ncmp, float64(ncmp)/float64(n*lg(n)), pcmp)
					}
				}
			}
		}
	}
	t.Logf("blqsort worst case: %.2f n lg n compares (%s)", worst, worstDesc)
	t.Logf("pdqsort worst case: %.2f n lg n compares (%s)", pdqWorst, pdqWorstDesc)
}

// McIlroy's "antiquicksort" adversary, as in the stdlib TestAdversary. Items are
// ids; the comparator assigns their values lazily to force bad pivots. Must
// degrade to heapsort, not O(n^2).
func TestAdversary(t *testing.T) {
	// mirrored resolves gas-gas comparisons the other way round. Plain McIlroy
	// is satisfied by the sorted-part probes in n-1 compares; the mirrored
	// variant makes them fail and attacks the partitioning instead.
	for _, mirrored := range []bool{false, true} {
		for _, size := range []int{100, 1000, 1025, 10000, 100000, 1000000} {
			maxcmp := size * lg(size) * 4
			gas := size - 1
			vals := make([]int, size)
			for i := range vals {
				vals[i] = gas
			}
			ncmp, nsolid, candidate := 0, 0, 0
			less := func(i, j int) bool {
				if ncmp >= maxcmp {
					t.Fatalf("size %d: used %d comparisons sorting adversary data", size, ncmp)
				}
				ncmp++
				if vals[i] == gas && vals[j] == gas {
					if (i == candidate) != mirrored {
						vals[i] = nsolid
					} else {
						vals[j] = nsolid
					}
					nsolid++
				}
				if vals[i] == gas {
					candidate = i
				} else if vals[j] == gas {
					candidate = j
				}
				return vals[i] < vals[j]
			}
			ids := make([]int, size)
			for i := range ids {
				ids[i] = i
			}
			// Call the partitioning core directly: the entry point's sorted-input
			// pre-check would otherwise satisfy the adversary in n-1 compares.
			sortrLess(ids, 0, size-1, make([]int, smallPart+1), lg(size), less)
			for i, id := range ids {
				if vals[id] != i {
					t.Fatalf("size %d: adversary data not fully sorted", size)
				}
			}
			t.Logf("mirrored=%v size %d: %d compares (%.2f n lg n)", mirrored, size, ncmp, float64(ncmp)/float64(size*lg(size)))
		}
	}
}

// Inconsistent comparators must not panic (golang.org/issue/14377) and,
// stricter than the stdlib test, must not lose or duplicate elements.
func TestNonDeterministicComparison(t *testing.T) {
	rng := rand.New(rand.NewSource(41))
	for _, n := range []int{2, 19, 100, 1025, 1026, 5000, 100000} {
		for range 5 {
			data := make([]int, n)
			for i := range data {
				data[i] = i
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("n=%d: panic with inconsistent comparator: %v", n, r)
					}
				}()
				SortLess(data, func(a, b int) bool { return rng.Intn(2) == 0 })
				SortFunc(data, func(a, b int) int { return rng.Intn(3) - 1 })
			}()
			slices.Sort(data)
			for i, v := range data {
				if v != i {
					t.Fatalf("n=%d: elements lost or duplicated with inconsistent comparator", n)
				}
			}
		}
	}
}

func FuzzSort(f *testing.F) {
	f.Add([]byte{3, 1, 2})
	f.Add(make([]byte, 64))
	f.Fuzz(func(t *testing.T, b []byte) {
		ints := make([]int8, len(b))
		for i, c := range b {
			ints[i] = int8(c)
		}
		want := slices.Clone(ints)
		slices.Sort(want)
		Sort(ints)
		if !slices.Equal(ints, want) {
			t.Fatalf("int8: got %v want %v", ints, want)
		}
		// Reinterpret 8-byte groups as float64 bits: NaN payloads, Infs, -0, subnormals.
		fl := make([]float64, len(b)/8)
		for i := range fl {
			var u uint64
			for j := range 8 {
				u = u<<8 | uint64(b[i*8+j])
			}
			fl[i] = math.Float64frombits(u)
		}
		wantF := slices.Clone(fl)
		slices.Sort(wantF)
		Sort(fl)
		if !nanEqual(fl, wantF) {
			t.Fatalf("float64: got %v want %v", fl, wantF)
		}
	})
}
