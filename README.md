# blqsort

A branchless quicksort for Go, ported from
[blqsort](https://github.com/chkas/blqsort) by Christof Kaser. It is built for
**data whose comparisons are unpredictable**, i.e. random data, where it is
about 2.5–3x faster than `slices.Sort` on amd64 (Intel) and 3–3.7x on arm64
(Apple M1 Pro) for large inputs. It is ahead from 100 elements up (1.4x on
Intel, 2.7x on M1 at 100 random ints), if the data is actually fresh; see
"Benchmarking small sorts" below.

```go
blqsort.Sort(xs)                                          // cmp.Ordered, like slices.Sort
blqsort.SortByKey(users, func(u User) int64 { return u.ID })
blqsort.SortLess(users, func(a, b User) bool { return a.ID < b.ID })
blqsort.SortFunc(users, cmpFn)                            // only fast with a branch-free cmpFn
```

None of the sorts are stable. They don't allocate. `Sort` orders NaNs first,
like `slices.Sort`, and `SortByKey` orders elements whose key is NaN first.

## Generated sorters for your own types

`SortByKey` and friends call your function for every comparison. For the
best speed on structs, generate a sorter specialized for your type and keys.
Add the generator to your module as a tool (Go 1.24 or later):

```bash
go get -tool github.com/dlm/blqsort/cmd/blqsortgen@v0.1.1
```

Then

```go
//go:generate go run github.com/dlm/blqsort/cmd/blqsortgen -type Person -by=-Age,Name.Last
```

writes a self-contained file (no dependency on this module) with
`func sortPersonByAgeDescLast(data []Person)`, sorting by `Age` descending,
then `Name.Last`. Multi-key comparisons are generated branch-free. Several
generated sorters can share a package. See `go doc
github.com/dlm/blqsort/cmd/blqsortgen` and `cmd/blqsortgen/example`. On
random data it was about 3.5x faster than `slices.SortFunc` for 8-byte
records with an integer key, and 1.4x for 56-byte records sorted by an int
and two strings.

## When to use it

Quicksort partitioning compares each element with a pivot. On random data
the outcome is a coin flip, so a branchy partition (as in `slices.Sort`'s
pdqsort) mispredicts about half the time. blqsort's partition never branches
on the comparison; it pays a fixed cost per element instead.

On data with regular structure (periodic values, interleaved or partly sorted
sequences) the CPU's branch predictor learns the pattern and branchy
partitioning becomes much cheaper, so `slices.Sort` can be faster there.
blqsort hands inputs made of a few long runs to `slices.Sort`, but not every
structured input is detected. It's also why `SortFunc` with a comparator
built from `cmp.Compare` is slow: that comparator itself branches on the
data. Use `SortByKey`, which keeps the comparison branch-free, or `SortLess`
with a simple less function.

## Results

Single-threaded, medians from `run-benchmarks.sh` in
[blqsort-experiments](https://github.com/dlm/blqsort-experiments) (Go 1.26):
Intel Core Ultra X7 358H pinned to one performance core, and Apple M1 Pro
(macOS can't pin). Milliseconds for 1M int64 (`BenchmarkLibraryShapes`):

| int64, 1M | | `slices.Sort` | **blqsort** | radix (zermelo) | sorty (1 goroutine) |
|---|---|---|---|---|---|
| random | Intel | 57.1 | 22.1 | 22.9 | 49.0 |
| | M1 Pro | 64.3 | 17.6 | 12.0 | 56.7 |
| 10 distinct values | Intel | 9.90 | 3.63 | 4.71 | 13.8 |
| | M1 Pro | 11.3 | 4.86 | 3.84 | 17.3 |
| sorted | Intel | 0.37 | 0.29 | 0.84 | 3.72 |
| | M1 Pro | 0.63 | 0.33 | 0.70 | 7.15 |
| reversed | Intel | 0.64 | 0.59 | 14.9 | 3.88 |
| | M1 Pro | 0.93 | 0.79 | 12.6 | 7.31 |
| `i % 8` | Intel | 2.02 | 2.15 | 4.57 | 6.90 |
| | M1 Pro | 2.99 | 3.12 | 3.70 | 10.1 |
| `i ^ 0xcccc` | Intel | 20.4 | 21.4 | 14.0 | 13.0 |
| | M1 Pro | 24.7 | 17.3 | 12.4 | 18.0 |

Radix numbers are with a caller-supplied buffer; without one it allocates n
elements per call. Random data, speedup over `slices.Sort`
(`BenchmarkLibraries`):

| | Intel | M1 Pro |
|---|---|---|
| int64, 100 / 1,000 / 10,000 elements (fresh inputs, `BenchmarkCrossover`) | 1.40x / 1.85x / 2.14x | 2.75x / 3.28x / 3.55x |
| int64, 100K / 1M / 10M | 2.40x / 2.57x / 2.59x | 3.67x / 3.60x / 3.64x |
| float64, 100K / 1M / 10M | 2.67x / 2.85x / 2.85x | 2.80x / 3.00x / 3.20x |

### Benchmarking small sorts

A benchmark that sorts the same small input over and over lets the CPU's
branch predictor learn that input. That hides exactly the mispredictions
blqsort avoids: on 1,000 random ints, `slices.Sort` is 4.5x faster (Intel)
or 3.6x faster (M1) on a repeated input than on fresh ones, while blqsort
gains less. With a repeated input the two look even up to about 1,000
elements; with fresh inputs blqsort is ahead at every size. Above roughly
10K-30K elements the predictor can't learn the input and both methods
agree. Go's own `sort` benchmarks repeat one input, as do most quick
benchmarks.

The C original uses branchy code on arm64 because branchless was slower on
M1 there; in Go, the branchless version is clearly ahead of `slices.Sort` on
M1. A branchy blqsort was not measured.

## Correctness

Tested against `slices.Sort` on many distributions and sizes, value-based
ports of the stdlib's sort tests (Bentley-McIlroy matrix, McIlroy's
adversary, inconsistent comparators), exhaustive checks of the sorting and
median networks, the race detector and fuzzing. The port differs from the C
original where the tests found problems (duplicate handling in small
partitions, lopsided-split detection, pivot sampling); see comments in
`blqsort.go`.

## Layout

- `blqsort.go`: the algorithm (hand-written); everything named `*_gen.go` is
  generated from it by `go generate` (`go run ./gen`, pure Go).
- `cmd/blqsortgen`: the generator for specialized sorters, with an example
  package that is regenerated and tested along with everything else.
- `internal/codegen`, `gen/`: the code generation shared by the repository
  and `blqsortgen`; `go generate ./...` regenerates everything (pure Go).

The measurements and the comparisons with other libraries live in a
separate repository,
[blqsort-experiments](https://github.com/dlm/blqsort-experiments), which
depends on a released version of this module.

## Credits and license

Ported from [chkas/blqsort](https://github.com/chkas/blqsort) (MIT,
Christof Kaser); the sorting networks are generated from its header
(`gen/third_party/blqsort.h`).

MIT; see [LICENSE](LICENSE).
