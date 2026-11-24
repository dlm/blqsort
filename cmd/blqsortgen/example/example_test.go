package example

import (
	"cmp"
	"math/rand"
	"slices"
	"testing"
)

func people(rng *rand.Rand, n int) []Person {
	names := []string{"ada", "bob", "cy", "dee", "eve"}
	p := make([]Person, n)
	for i := range p {
		p[i] = Person{
			ID:    int64(i),
			Name:  Name{First: names[rng.Intn(len(names))], Last: names[rng.Intn(len(names))]},
			Age:   rng.Intn(90),
			Score: rng.Float64(),
		}
	}
	return p
}

// check sorts a copy of data with sort, and checks the order by key and that
// the result is a permutation (IDs are unique).
func check[T any](t *testing.T, name string, data []T, sort func([]T), key func(a, b T) int, id func(T) int64) {
	t.Helper()
	got := slices.Clone(data)
	sort(got)
	if !slices.IsSortedFunc(got, key) {
		t.Fatalf("%s n=%d: not sorted", name, len(data))
	}
	ids := make([]int64, len(got))
	for i, x := range got {
		ids[i] = id(x)
	}
	slices.Sort(ids)
	for i, x := range ids {
		if x != int64(i) {
			t.Fatalf("%s n=%d: elements lost or duplicated", name, len(data))
		}
	}
}

func TestGenerated(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	byAgeDescName := func(a, b Person) int {
		return cmp.Or(cmp.Compare(b.Age, a.Age), cmp.Compare(a.Name.Last, b.Name.Last), cmp.Compare(a.Name.First, b.Name.First))
	}
	byScore := func(a, b Person) int { return cmp.Compare(a.Score, b.Score) }
	byID := func(a, b *Person) int { return cmp.Compare(a.ID, b.ID) }
	for _, n := range []int{0, 1, 2, 18, 19, 1025, 5000, 100000} {
		p := people(rng, n)
		check(t, "sortPersonByAgeDescLastFirst", p, sortPersonByAgeDescLastFirst, byAgeDescName, func(x Person) int64 { return x.ID })
		check(t, "sortPeopleByScore", p, sortPeopleByScore, byScore, func(x Person) int64 { return x.ID })
		ptrs := make([]*Person, n)
		for i := range p {
			ptrs[i] = &p[i]
		}
		rng.Shuffle(n, func(i, j int) { ptrs[i], ptrs[j] = ptrs[j], ptrs[i] })
		check(t, "sortPersonPointers", ptrs, sortPersonPointers, byID, func(x *Person) int64 { return x.ID })
	}
}

func BenchmarkGenerated(b *testing.B) {
	p := people(rand.New(rand.NewSource(42)), 1_000_000)
	data := make([]Person, len(p))
	byAgeDescName := func(a, b Person) int {
		return cmp.Or(cmp.Compare(b.Age, a.Age), cmp.Compare(a.Name.Last, b.Name.Last), cmp.Compare(a.Name.First, b.Name.First))
	}
	for _, c := range []struct {
		name string
		sort func([]Person)
	}{
		{"slices.SortFunc", func(x []Person) { slices.SortFunc(x, byAgeDescName) }},
		{"generated", sortPersonByAgeDescLastFirst},
	} {
		b.Run(c.name, func(b *testing.B) {
			for b.Loop() {
				b.StopTimer()
				copy(data, p)
				b.StartTimer()
				c.sort(data)
			}
		})
	}
}
