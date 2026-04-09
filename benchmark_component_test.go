package xsearch

import "testing"

func BenchmarkComponent(b *testing.B) {
	b.Run("bloom/add", func(b *testing.B) {
		bloom := NewBloom(1<<16, 16)
		terms := []string{
			"atlas-reserve", "northwind-studio", "evergreen-craft", "harbor-select",
			"cinder-market", "solstice-cellar", "mariner-summit", "bluebird-heritage",
		}
		i := 0
		b.ReportAllocs()
		for b.Loop() {
			bloom.Add(terms[i&7])
			i++
		}
	})

	b.Run("bloom/may_contain_hit", func(b *testing.B) {
		bloom := NewBloom(1<<16, 16)
		bloom.Add("atlas-reserve")
		b.ReportAllocs()
		for b.Loop() {
			bloom.MayContain("atlas-reserve")
		}
	})

	b.Run("bloom/may_contain_miss", func(b *testing.B) {
		bloom := NewBloom(1<<16, 16)
		bloom.Add("atlas-reserve")
		b.ReportAllocs()
		for b.Loop() {
			bloom.MayContain("zzqxv")
		}
	})

	b.Run("extract_trigrams/allocating_medium", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			extractNormalizedTrigrams(nil, "atlas reserve coastal")
		}
	})

	b.Run("extract_trigrams/buffered_medium", func(b *testing.B) {
		var buf [32]string
		b.ReportAllocs()
		for b.Loop() {
			extractNormalizedTrigrams(buf[:0], "atlas reserve coastal")
		}
	})

	b.Run("extract_trigrams/buffered_long", func(b *testing.B) {
		var buf [64]string
		b.ReportAllocs()
		for b.Loop() {
			extractNormalizedTrigrams(buf[:0], "northwind smoky whisky heritage highland single-origin")
		}
	})

	b.Run("tokenize/ascii", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			tokenize("Northwind smoky whisky heritage highland reserve")
		}
	})

	b.Run("tokenize/mixed_case_unicode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			tokenize("São PAULO Reserve Nº42 Citrus Blend")
		}
	})

	for _, size := range []benchmarkSize{benchmarkSizes[0], benchmarkSizes[1]} {
		b.Run("bm25/"+size.name, func(b *testing.B) {
			corpus := benchmarkCorpusFor(size.docs)
			engine, err := benchmarkEngineFor(size.docs, benchmarkEngineVariant{name: "default"})
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				engine.bm25.Search(corpus.exactQuery)
			}
		})
		b.Run("ngram/"+size.name, func(b *testing.B) {
			corpus := benchmarkCorpusFor(size.docs)
			engine, err := benchmarkEngineFor(size.docs, benchmarkEngineVariant{name: "default"})
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				engine.ngram.Search(corpus.typoQuery)
			}
		})
	}

	for _, size := range []benchmarkSize{benchmarkSizes[0], benchmarkSizes[1]} {
		b.Run("fallback/exact/"+size.name, func(b *testing.B) {
			corpus := benchmarkCorpusFor(size.docs)
			engine, err := benchmarkEngineFor(size.docs, benchmarkEngineVariant{name: "fallback", opts: []Option{WithFallbackField("category")}})
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				engine.fallback.exact(corpus.fallbackExactQuery)
			}
		})
		b.Run("fallback/best/"+size.name, func(b *testing.B) {
			corpus := benchmarkCorpusFor(size.docs)
			engine, err := benchmarkEngineFor(size.docs, benchmarkEngineVariant{name: "fallback", opts: []Option{WithFallbackField("category")}})
			if err != nil {
				b.Fatal(err)
			}
			queryGrams := extractNormalizedTrigrams(nil, corpus.fallbackTypoQuery)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				engine.fallback.best(corpus.fallbackTypoQuery, queryGrams)
			}
		})
	}
}
