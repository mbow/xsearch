package xsearch

import (
	"runtime"
	"testing"
)

var benchmarkBuildVariants = []benchmarkEngineVariant{
	{name: "default"},
	{name: "bloom", opts: []Option{WithBloom(96)}},
	{name: "fallback", opts: []Option{WithFallbackField("category")}},
	{name: "full", opts: []Option{WithBloom(96), WithFallbackField("category")}},
}

var benchmarkSnapshotVariants = []benchmarkEngineVariant{
	{name: "default"},
	{name: "full", opts: []Option{WithBloom(96), WithFallbackField("category")}},
}

func BenchmarkBuildEngine(b *testing.B) {
	for _, variant := range benchmarkBuildVariants {
		for _, size := range benchmarkSizes {
			corpus := benchmarkCorpusFor(size.docs)
			b.Run(variant.name+"/"+size.name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					engine, err := New(corpus.items, variant.opts...)
					if err != nil {
						b.Fatal(err)
					}
					runtime.KeepAlive(engine)
				}
			})
		}
	}
}

func BenchmarkSnapshotEncode(b *testing.B) {
	for _, variant := range benchmarkSnapshotVariants {
		for _, size := range benchmarkSizes {
			corpus := benchmarkCorpusFor(size.docs)
			engine, err := New(corpus.items, variant.opts...)
			if err != nil {
				b.Fatal(err)
			}
			snapshot, err := engine.Snapshot()
			if err != nil {
				b.Fatal(err)
			}
			b.Run(variant.name+"/"+size.name, func(b *testing.B) {
				b.SetBytes(int64(len(snapshot)))
				b.ReportAllocs()
				for b.Loop() {
					data, err := engine.Snapshot()
					if err != nil {
						b.Fatal(err)
					}
					runtime.KeepAlive(data)
				}
			})
		}
	}
}

func BenchmarkSnapshotLoad(b *testing.B) {
	for _, variant := range benchmarkSnapshotVariants {
		for _, size := range benchmarkSizes {
			corpus := benchmarkCorpusFor(size.docs)
			engine, err := New(corpus.items, variant.opts...)
			if err != nil {
				b.Fatal(err)
			}
			snapshot, err := engine.Snapshot()
			if err != nil {
				b.Fatal(err)
			}
			b.Run(variant.name+"/"+size.name, func(b *testing.B) {
				b.SetBytes(int64(len(snapshot)))
				b.ReportAllocs()
				for b.Loop() {
					loaded, err := NewFromSnapshot(snapshot, corpus.items)
					if err != nil {
						b.Fatal(err)
					}
					runtime.KeepAlive(loaded)
				}
			})
		}
	}
}
