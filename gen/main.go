// Command gen regenerates all generated code (files named *_gen.go) of
// blqsort, and the sort source that cmd/blqsortgen embeds. Run it from the
// repository root with go generate or go run ./gen. Only blqsort.go is
// edited by hand.
//
// It reads the sorting networks from third_party/blqsort.h (chkas/blqsort,
// MIT).
package main

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"

	"github.com/dlm/blqsort/internal/codegen"
)

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gen: "+format+"\n", args...)
	os.Exit(1)
}

func read(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		fail("%v", err)
	}
	return string(b)
}

// write gofmts code and writes it to path.
func write(path, code string) {
	src, err := format.Source([]byte(code))
	if err != nil {
		fail("%s: %v", path, err)
	}
	if err := os.WriteFile(path, src, 0o644); err != nil {
		fail("%v", err)
	}
}

// helpersBody returns the shared, non-specialized helpers of blqsort.go
// (constants, b2i, pivotJitter, ...: everything before heapSort).
func helpersBody(src string) string {
	imp := strings.Index(src, "import (")
	return src[strings.Index(src[imp:], ")\n")+imp+2 : strings.Index(src, "func heapSort")]
}

// render renders sp or exits.
func render(sp codegen.Spec, srcs []string) string {
	out, err := sp.Render(srcs)
	if err != nil {
		fail("%v", err)
	}
	return out
}

// declsFrom returns a generated Go file from its first top-level func on,
// i.e. without header, package clause and imports.
func declsFrom(src string) string {
	return src[strings.Index(src, "\nfunc ")+1:]
}

const (
	lessDoc = `SortLess sorts data in ascending order as determined by less, which
reports whether a sorts before b. A simple less such as a.ID < b.ID compiles
to branch-free code, which is what makes this fast; avoid branchy less
functions such as a.X < b.X || (a.X == b.X && a.Y < b.Y).`

	byKeyDoc = `SortByKey sorts data in ascending order of key(x), comparing keys with <.
The comparison is done here, branch-free, so key only needs to extract the
key (e.g. a field). The key of the pivot is computed once per partition,
so key is called about once per element per partition pass.
NaN keys are not ordered.`
)

func main() {
	if _, err := os.Stat("blqsort.go"); err != nil {
		fail("run from the repository root (go generate does this)")
	}
	netHeader := read(filepath.Join("gen", "third_party", "blqsort.h"))
	blq := read("blqsort.go")

	// --- Library: package blqsort ---
	write("networks_gen.go", genNetworks(netHeader, "blqsort", false))
	write("networks_func_gen.go", genNetworks(netHeader, "blqsort", true))
	write("func_gen.go", deriveFunc(blq))
	blqSrc := []string{read("func_gen.go"), read("networks_func_gen.go")}
	lib := []struct {
		out string
		sp  codegen.Spec
	}{
		{"less_gen.go", codegen.Spec{Pkg: "blqsort", TypeParams: "T any", Param: "less func(a, b T) bool",
			Suffix: "Less", Less: "less({a}, {b})", Doc: lessDoc}},
		{"bykey_gen.go", codegen.Spec{Pkg: "blqsort", TypeParams: "T any, K cmp.Ordered", Param: "key func(T) K",
			Suffix: "ByKey", Less: "key({a}) < key({b})",
			PivotLess: "key({a}) < pk", PivotInit: "pk := key(piv)", Doc: byKeyDoc}},
	}
	for _, j := range lib {
		write(j.out, render(j.sp, blqSrc))
	}

	// --- cmd/blqsortgen: the comparator sort source it specializes ---
	// The generic cmp.Ordered helpers (s2, NaN handling) are not used by the
	// comparator variant and would collide with its names.
	hb, err := codegen.DropGenericFuncs(helpersBody(blq))
	if err != nil {
		fail("%v", err)
	}
	src := hb + "\n" + declsFrom(read("func_gen.go")) + "\n" + declsFrom(read("networks_func_gen.go"))
	if err := os.WriteFile(filepath.Join("cmd", "blqsortgen", "sortsource.txt"), []byte(src), 0o644); err != nil {
		fail("%v", err)
	}
	// Generated files carry the license (go:embed can't reach ../../LICENSE).
	if err := os.WriteFile(filepath.Join("cmd", "blqsortgen", "license.txt"), []byte(read("LICENSE")), 0o644); err != nil {
		fail("%v", err)
	}
}
