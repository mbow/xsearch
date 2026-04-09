package xsearch

import "testing"

type benchmarkSearchCase struct {
	name       string
	variant    benchmarkEngineVariant
	query      func(*benchmarkCorpus) string
	withScorer bool
}

var benchmarkSearchCases = []benchmarkSearchCase{
	{
		name:    "exact/default",
		variant: benchmarkEngineVariant{name: "default"},
		query:   func(c *benchmarkCorpus) string { return c.exactQuery },
	},
	{
		name:    "exact/bloom",
		variant: benchmarkEngineVariant{name: "bloom", opts: []Option{WithBloom(96)}},
		query:   func(c *benchmarkCorpus) string { return c.exactQuery },
	},
	{
		name:    "prefix/default",
		variant: benchmarkEngineVariant{name: "default"},
		query:   func(c *benchmarkCorpus) string { return c.prefixQuery },
	},
	{
		name:    "typo/default",
		variant: benchmarkEngineVariant{name: "default"},
		query:   func(c *benchmarkCorpus) string { return c.typoQuery },
	},
	{
		name:    "typo/bloom",
		variant: benchmarkEngineVariant{name: "bloom", opts: []Option{WithBloom(96)}},
		query:   func(c *benchmarkCorpus) string { return c.typoQuery },
	},
	{
		name:    "miss/default",
		variant: benchmarkEngineVariant{name: "default"},
		query:   func(c *benchmarkCorpus) string { return c.missQuery },
	},
	{
		name:    "miss_bloom/bloom",
		variant: benchmarkEngineVariant{name: "bloom", opts: []Option{WithBloom(96)}},
		query:   func(c *benchmarkCorpus) string { return c.missQuery },
	},
	{
		name:    "fallback_exact/fallback",
		variant: benchmarkEngineVariant{name: "fallback", opts: []Option{WithFallbackField("category")}},
		query:   func(c *benchmarkCorpus) string { return c.fallbackExactQuery },
	},
	{
		name:    "fallback_typo/fallback",
		variant: benchmarkEngineVariant{name: "fallback", opts: []Option{WithFallbackField("category")}},
		query:   func(c *benchmarkCorpus) string { return c.fallbackTypoQuery },
	},
	{
		name:       "scorer/default",
		variant:    benchmarkEngineVariant{name: "default"},
		query:      func(c *benchmarkCorpus) string { return c.commonQuery },
		withScorer: true,
	},
	{
		name:    "limit_10/default",
		variant: benchmarkEngineVariant{name: "default", opts: []Option{WithLimit(10)}},
		query:   func(c *benchmarkCorpus) string { return c.commonQuery },
	},
	{
		name:    "limit_100/wide",
		variant: benchmarkEngineVariant{name: "wide", opts: []Option{WithLimit(100)}},
		query:   func(c *benchmarkCorpus) string { return c.commonQuery },
	},
	{
		name:    "multiword/default",
		variant: benchmarkEngineVariant{name: "default"},
		query:   func(c *benchmarkCorpus) string { return c.multiWordQuery },
	},
}

var benchmarkParallelCases = []benchmarkSearchCase{
	{
		name:    "exact/default",
		variant: benchmarkEngineVariant{name: "default"},
		query:   func(c *benchmarkCorpus) string { return c.exactQuery },
	},
	{
		name:    "typo/default",
		variant: benchmarkEngineVariant{name: "default"},
		query:   func(c *benchmarkCorpus) string { return c.typoQuery },
	},
	{
		name:    "miss/default",
		variant: benchmarkEngineVariant{name: "default"},
		query:   func(c *benchmarkCorpus) string { return c.missQuery },
	},
	{
		name:    "miss_bloom/bloom",
		variant: benchmarkEngineVariant{name: "bloom", opts: []Option{WithBloom(96)}},
		query:   func(c *benchmarkCorpus) string { return c.missQuery },
	},
}

func BenchmarkSearch(b *testing.B) {
	for _, searchCase := range benchmarkSearchCases {
		for _, size := range benchmarkSizes {
			b.Run(searchCase.name+"/"+size.name, func(b *testing.B) {
				corpus := benchmarkCorpusFor(size.docs)
				engine, err := benchmarkEngineFor(size.docs, searchCase.variant)
				if err != nil {
					b.Fatal(err)
				}
				query := searchCase.query(corpus)
				var searchOpts []SearchOption
				if searchCase.withScorer {
					searchOpts = []SearchOption{WithScoring(corpus.scorer)}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					engine.Search(query, searchOpts...)
				}
			})
		}
	}
}

func BenchmarkParallelSearch(b *testing.B) {
	size := benchmarkSizes[len(benchmarkSizes)-1]

	for _, searchCase := range benchmarkParallelCases {
		b.Run(searchCase.name+"/"+size.name, func(b *testing.B) {
			corpus := benchmarkCorpusFor(size.docs)
			engine, err := benchmarkEngineFor(size.docs, searchCase.variant)
			if err != nil {
				b.Fatal(err)
			}
			query := searchCase.query(corpus)
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					engine.Search(query)
				}
			})
		})
	}
}
