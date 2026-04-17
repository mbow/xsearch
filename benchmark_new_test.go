package xsearch

import (
	"fmt"
	"testing"
)

func BenchmarkFold_ASCII(b *testing.B) {
	s := "plain ascii drink name with no marks"
	for i := 0; i < b.N; i++ {
		_ = Fold(s)
	}
}

func BenchmarkFold_Latin1(b *testing.B) {
	s := "Moët & Chandon Impérial Brut Rosé"
	for i := 0; i < b.N; i++ {
		_ = Fold(s)
	}
}

type benchItem struct {
	id, name, group string
}

func (b benchItem) SearchID() string { return b.id }
func (b benchItem) SearchFields() []Field {
	return []Field{
		{Name: "name", Values: []string{b.name}, Weight: 1.0},
		{Name: "comparison_group", Values: []string{b.group}, Weight: 0.8},
	}
}

func newBenchEngine(b *testing.B, foldOn bool) *Engine {
	b.Helper()
	items := make([]benchItem, 1024)
	for i := range items {
		items[i] = benchItem{
			id:    fmt.Sprintf("d%d", i),
			name:  fmt.Sprintf("Drink Number %d", i),
			group: "lager",
		}
	}
	opts := []Option{WithLimit(25), WithFallbackField("comparison_group")}
	if foldOn {
		opts = append(opts, WithUnicodeFold())
	}
	e, err := New(items, opts...)
	if err != nil {
		b.Fatal(err)
	}
	return e
}

func BenchmarkSearch_UnicodeFold_Off(b *testing.B) {
	e := newBenchEngine(b, false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.Search("drink number 42")
	}
}

func BenchmarkSearch_UnicodeFold_On(b *testing.B) {
	e := newBenchEngine(b, true)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.Search("drink number 42")
	}
}

func BenchmarkSearchWithFallback_PrimaryHits(b *testing.B) {
	e := newBenchEngine(b, false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = e.SearchWithFallback("drink number 42", []string{"lager", "beer"})
	}
}

func BenchmarkSearchWithFallback_CascadeHits(b *testing.B) {
	e := newBenchEngine(b, false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = e.SearchWithFallback("nonexistent-xyz", []string{"lager", "beer"})
	}
}

func BenchmarkWithFilter_NilFilter(b *testing.B) {
	e := newBenchEngine(b, false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.Search("drink number 42", WithFilter(nil))
	}
}

func BenchmarkWithFilter_KeepHalf(b *testing.B) {
	e := newBenchEngine(b, false)
	keepHalf := func(id string) bool {
		return len(id) > 2 // arbitrary, keeps most
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = e.Search("drink number 42", WithFilter(keepHalf))
	}
}
