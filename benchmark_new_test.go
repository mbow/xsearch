package xsearch

import (
	"fmt"
	"testing"
)

func BenchmarkFold_ASCII(b *testing.B) {
	b.ReportAllocs()
	s := "plain ascii drink name with no marks"
	for b.Loop() {
		Fold(s)
	}
}

func BenchmarkFold_Latin1(b *testing.B) {
	b.ReportAllocs()
	s := "Moët & Chandon Impérial Brut Rosé"
	for b.Loop() {
		Fold(s)
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
	b.ReportAllocs()
	for b.Loop() {
		e.Search("drink number 42")
	}
}

func BenchmarkSearch_UnicodeFold_On(b *testing.B) {
	e := newBenchEngine(b, true)
	b.ReportAllocs()
	for b.Loop() {
		e.Search("drink number 42")
	}
}

func BenchmarkSearchWithFallback_PrimaryHits(b *testing.B) {
	e := newBenchEngine(b, false)
	b.ReportAllocs()
	for b.Loop() {
		e.SearchWithFallback("drink number 42", []string{"lager", "beer"})
	}
}

func BenchmarkSearchWithFallback_CascadeHits(b *testing.B) {
	e := newBenchEngine(b, false)
	b.ReportAllocs()
	for b.Loop() {
		e.SearchWithFallback("nonexistent-xyz", []string{"lager", "beer"})
	}
}

func BenchmarkWithFilter_NilFilter(b *testing.B) {
	e := newBenchEngine(b, false)
	b.ReportAllocs()
	for b.Loop() {
		e.Search("drink number 42", WithFilter(nil))
	}
}

func BenchmarkWithFilter_KeepMost(b *testing.B) {
	e := newBenchEngine(b, false)
	keepMost := func(id string) bool {
		return len(id) > 2 // keeps all but d0..d9
	}
	b.ReportAllocs()
	for b.Loop() {
		e.Search("drink number 42", WithFilter(keepMost))
	}
}
