# Prefix Cache Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an opt-in `WithPrefixCache(prefixes)` option to xsearch that precomputes search results at construction time, restoring ~12ns cached prefix lookups.

**Architecture:** New `prefixCacheKeys` field on `engineConfig`, new `prefixCache map[string][]Result` on `Engine`. Built at the end of `newEngineFromPrepared()` by running the internal search pipeline for each prefix. `Search()` checks the cache first (bypassed when a `Scorer` is present). Consumer (htmx-xsearch) extracts prefixes from product data and passes them via the option.

**Tech Stack:** Go 1.26, xsearch lib, htmx-xsearch sample app, benchstat

**Spec:** `docs/superpowers/specs/2026-04-09-prefix-cache-design.md`

---

### Task 1: Add `WithPrefixCache` option to xsearch config

**Files:**
- Modify: `config.go`
- Modify: `xsearch_test.go`

- [ ] **Step 1: Write the failing test**

Add to `xsearch_test.go`:

```go
func TestPrefixCacheHit(t *testing.T) {
	items := testItems()
	prefixes := []string{"b", "bu", "n", "ni"}

	uncached, err := New(items)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := New(items, WithPrefixCache(prefixes))
	if err != nil {
		t.Fatal(err)
	}

	for _, prefix := range prefixes {
		want := uncached.Search(prefix)
		got := cached.Search(prefix)
		if len(got) != len(want) {
			t.Fatalf("prefix %q: got %d results, want %d", prefix, len(got), len(want))
		}
		for i := range want {
			if got[i].ID != want[i].ID {
				t.Fatalf("prefix %q result %d: got %q, want %q", prefix, i, got[i].ID, want[i].ID)
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestPrefixCacheHit -v`
Expected: FAIL — `WithPrefixCache` is not defined.

- [ ] **Step 3: Add the option constant, config field, and option function**

In `config.go`, add the new option kind after `optionFallbackField`:

```go
const (
	optionScorer optionKind = iota + 1
	optionBloom
	optionBM25
	optionAlpha
	optionLimit
	optionFallbackField
	optionPrefixCache
)
```

Add the config field to `engineConfig`:

```go
type engineConfig struct {
	bloomBitsPerItem int
	bm25K1           float64
	bm25B            float64
	alpha            float64
	limit            int
	fallbackField    string
	prefixCacheKeys  []string
}
```

Add the option function at the end of `config.go`:

```go
// WithPrefixCache precomputes search results for the given queries during
// engine construction. Cached queries return in O(1) with zero allocations.
// Typical usage: pass all 1-2 character prefixes of the primary search field.
func WithPrefixCache(prefixes []string) Option {
	return Option{
		kind: optionPrefixCache,
		apply: func(c *engineConfig) {
			c.prefixCacheKeys = prefixes
		},
	}
}
```

Allow `WithPrefixCache` with `NewFromSnapshot` by adding `optionPrefixCache` to the allowed list in `validateForSnapshotLoad`:

```go
func (o Option) validateForSnapshotLoad() error {
	switch o.kind {
	case optionAlpha, optionLimit, optionPrefixCache:
		return nil
	// ... rest unchanged
	}
}
```

- [ ] **Step 4: Run test to verify it still fails (option exists, but cache not built)**

Run: `go test -run TestPrefixCacheHit -v`
Expected: Compiles but test passes trivially (no cache means normal search path is used, which returns the same results). This is expected — the test validates correctness, not the fast path. We'll add a benchmark later to prove the cache is hit.

- [ ] **Step 5: Commit**

```bash
git add config.go xsearch_test.go
git commit -m "feat: add WithPrefixCache option (config only, not yet wired)"
```

---

### Task 2: Build the prefix cache during engine construction

**Files:**
- Modify: `xsearch.go`

- [ ] **Step 1: Write the failing test for cache bypass with scorer**

Add to `xsearch_test.go`:

```go
func TestPrefixCacheBypassWithScorer(t *testing.T) {
	items := testItems()
	engine, err := New(items, WithPrefixCache([]string{"bud"}))
	if err != nil {
		t.Fatal(err)
	}

	// Without scorer — should use cache (same results as uncached).
	uncached, _ := New(items)
	cachedResults := engine.Search("bud")
	uncachedResults := uncached.Search("bud")
	if len(cachedResults) != len(uncachedResults) {
		t.Fatalf("cached count %d != uncached count %d", len(cachedResults), len(uncachedResults))
	}

	// With scorer — should bypass cache and apply scoring.
	scorer := mapScorer{0: 100, 1: 0}
	scoredResults := engine.Search("bud", WithScoring(scorer))
	if len(scoredResults) == 0 {
		t.Fatal("expected scored results")
	}
	// Scored results should differ from cached (scorer reorders).
	if len(scoredResults) == len(cachedResults) && scoredResults[0].Score == cachedResults[0].Score {
		t.Fatal("scorer should produce different scores than cached results")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run TestPrefixCacheBypassWithScorer -v`
Expected: FAIL or unexpected results because cache is not built yet (passes trivially through normal path).

- [ ] **Step 3: Add `prefixCache` field to Engine and build it in `newEngineFromPrepared`**

In `xsearch.go`, add the field to `Engine`:

```go
type Engine struct {
	cfg         engineConfig
	items       []preparedItem
	idToDoc     map[string]int
	ordered     []string
	bloom       *Bloom
	bm25        *bm25Index
	ngram       *ngramIndex
	fallback    *fallbackIndex
	prefixCache map[string][]Result
}
```

Add a private method that runs the search pipeline without checking the cache. At the bottom of `xsearch.go`, before `computeHighlights`:

```go
// searchInternal runs the full search pipeline without checking the prefix cache.
func (e *Engine) searchInternal(query string) []Result {
	var sCfg searchConfig
	query = normalizeQuery(query)
	if query == "" {
		return nil
	}

	if docs, ok := e.fallback.exact(query); ok {
		return e.resultsForCandidates(query, fallbackCandidates(docs, nil), sCfg)
	}

	var trigramBuf [16]string
	queryGrams := extractNormalizedTrigrams(trigramBuf[:0], query)
	if len(queryGrams) > 0 {
		slices.Sort(queryGrams)
		queryGrams = slices.Compact(queryGrams)
	}

	var direct map[int]scoredCandidate

	if len(queryGrams) == 0 {
		if bm25Results := e.bm25.Search(query); len(bm25Results) > 0 {
			direct = make(map[int]scoredCandidate, len(bm25Results))
			for _, result := range bm25Results {
				direct[result.Doc] = scoredCandidate{
					doc:       result.Doc,
					relevance: result.Score,
					matchType: MatchDirect,
				}
			}
		} else {
			if ngramResults := e.ngram.Search(query); len(ngramResults) > 0 {
				direct = make(map[int]scoredCandidate, len(ngramResults))
				for _, result := range ngramResults {
					direct[result.Doc] = scoredCandidate{
						doc:       result.Doc,
						relevance: result.Score,
						matchType: MatchDirect,
					}
				}
			}
		}
	} else {
		if e.bloom != nil {
			if !slices.ContainsFunc(queryGrams, e.bloom.MayContain) {
				if docs, ok := e.fallback.best(query, queryGrams); ok {
					return e.resultsForCandidates(query, fallbackCandidates(docs, nil), sCfg)
				}
				return nil
			}
		}

		if bm25Results := e.bm25.Search(query); len(bm25Results) > 0 {
			direct = make(map[int]scoredCandidate, len(bm25Results))
			for _, result := range bm25Results {
				direct[result.Doc] = scoredCandidate{
					doc:       result.Doc,
					relevance: result.Score,
					matchType: MatchDirect,
				}
			}
		} else {
			if ngramResults := e.ngram.SearchWithGrams(queryGrams); len(ngramResults) > 0 {
				direct = make(map[int]scoredCandidate, len(ngramResults))
				for _, result := range ngramResults {
					direct[result.Doc] = scoredCandidate{
						doc:       result.Doc,
						relevance: result.Score,
						matchType: MatchDirect,
					}
				}
			}
		}
	}

	if len(direct) < defaultMinDirectResults {
		if docs, ok := e.fallback.best(query, queryGrams); ok {
			fallback := fallbackCandidates(docs, direct)
			if direct == nil {
				direct = fallback
			} else {
				maps.Copy(direct, fallback)
			}
		}
	}

	return e.resultsForCandidates(query, direct, sCfg)
}
```

**Important:** This duplicates the body of `Search()` minus the cache check and scorer setup. To keep things DRY, refactor `Search()` to delegate to `searchInternal` after handling cache and scorer. Replace the current `Search()` method body with:

```go
func (e *Engine) Search(query string, opts ...SearchOption) []Result {
	var sCfg searchConfig
	for _, opt := range opts {
		opt(&sCfg)
	}

	query = normalizeQuery(query)
	if query == "" {
		return nil
	}

	// Prefix cache fast path — only when no scorer is applied.
	if e.prefixCache != nil && sCfg.scorer == nil {
		if cached, ok := e.prefixCache[query]; ok {
			return cached
		}
	}

	return e.searchWithConfig(query, sCfg)
}
```

Then rename the existing search body (everything after normalizeQuery in the current `Search()`) to `searchWithConfig(query string, sCfg searchConfig) []Result`. This method takes a pre-normalized query:

```go
func (e *Engine) searchWithConfig(query string, sCfg searchConfig) []Result {
	if docs, ok := e.fallback.exact(query); ok {
		return e.resultsForCandidates(query, fallbackCandidates(docs, nil), sCfg)
	}

	// ... rest of existing Search() body after normalizeQuery, unchanged ...
}
```

And `searchInternal` becomes simply:

```go
func (e *Engine) searchInternal(query string) []Result {
	return e.searchWithConfig(normalizeQuery(query), searchConfig{})
}
```

Now build the cache at the end of `newEngineFromPrepared()`:

```go
func newEngineFromPrepared(items []preparedItem, cfg engineConfig) *Engine {
	e := &Engine{
		// ... existing construction unchanged ...
	}
	// ... existing index building unchanged ...

	if len(cfg.prefixCacheKeys) > 0 {
		e.buildPrefixCache(cfg.prefixCacheKeys)
	}
	return e
}
```

Add the build method:

```go
func (e *Engine) buildPrefixCache(keys []string) {
	seen := make(map[string]struct{}, len(keys))
	e.prefixCache = make(map[string][]Result, len(keys))
	for _, key := range keys {
		normalized := normalizeQuery(key)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		if results := e.searchInternal(normalized); len(results) > 0 {
			e.prefixCache[normalized] = results
		}
	}
}
```

- [ ] **Step 4: Run all tests**

Run: `go test ./... -v -count=1`
Expected: All tests pass including `TestPrefixCacheHit` and `TestPrefixCacheBypassWithScorer`.

- [ ] **Step 5: Commit**

```bash
git add xsearch.go xsearch_test.go
git commit -m "feat: build prefix cache during engine construction

WithPrefixCache precomputes results for given queries at build time.
Cache is checked first in Search() and bypassed when a Scorer is present."
```

---

### Task 3: Add prefix cache normalization test

**Files:**
- Modify: `xsearch_test.go`

- [ ] **Step 1: Write the test**

```go
func TestPrefixCacheNormalization(t *testing.T) {
	items := testItems()
	// Mixed case and duplicates should be normalized and deduped.
	engine, err := New(items, WithPrefixCache([]string{"B", "b", "BU", "bu", " b "}))
	if err != nil {
		t.Fatal(err)
	}
	// All should resolve to the same normalized keys.
	r1 := engine.Search("b")
	r2 := engine.Search("B")
	if len(r1) == 0 {
		t.Fatal("expected results for 'b'")
	}
	if len(r1) != len(r2) {
		t.Fatalf("normalized queries should return same results: %d vs %d", len(r1), len(r2))
	}
}
```

- [ ] **Step 2: Run test**

Run: `go test -run TestPrefixCacheNormalization -v`
Expected: PASS — normalizeQuery lowercases and trims, and `buildPrefixCache` deduplicates.

- [ ] **Step 3: Commit**

```bash
git add xsearch_test.go
git commit -m "test: add prefix cache normalization and dedup test"
```

---

### Task 4: Add prefix cache benchmark to xsearch

**Files:**
- Modify: `benchmark_search_test.go`
- Modify: `benchmark_fixture_test.go`

- [ ] **Step 1: Add `cachedPrefixQuery` to the benchmark corpus**

In `benchmark_fixture_test.go`, add a field to `benchmarkCorpus`:

```go
type benchmarkCorpus struct {
	items               []benchmarkItem
	exactQuery          string
	prefixQuery         string
	cachedPrefixQuery   string // single-char prefix for cache hit benchmarks
	typoQuery           string
	missQuery           string
	fallbackExactQuery  string
	fallbackTypoQuery   string
	commonQuery         string
	multiWordQuery      string
	scorer              benchmarkScorer
	negativeBloomProbe  string
}
```

In `newBenchmarkCorpus`, set it after the existing `prefixQuery` assignment:

```go
	return &benchmarkCorpus{
		items:              items,
		exactQuery:         exactName,
		prefixQuery:        benchmarkPrefixQuery(items[0].fields[0].Values[0]),
		cachedPrefixQuery:  strings.ToLower(items[0].fields[0].Values[0][:1]),
		// ... rest unchanged ...
	}
```

Add a helper to extract prefixes from benchmark items. Add at the bottom of `benchmark_fixture_test.go`:

```go
func benchmarkPrefixes(items []benchmarkItem) []string {
	seen := make(map[string]struct{})
	for _, item := range items {
		name := strings.ToLower(item.fields[0].Values[0])
		if len(name) >= 1 {
			seen[name[:1]] = struct{}{}
		}
		if len(name) >= 2 {
			seen[name[:2]] = struct{}{}
		}
	}
	prefixes := make([]string, 0, len(seen))
	for p := range seen {
		prefixes = append(prefixes, p)
	}
	return prefixes
}
```

Add a "cached" engine variant to `benchmarkEngineFor`. Replace the function to handle prefix cache variants:

```go
func benchmarkEngineFor(docs int, variant benchmarkEngineVariant) (*Engine, error) {
	key := fmt.Sprintf("%d/%s", docs, variant.name)

	benchmarkEngineCache.mu.Lock()
	defer benchmarkEngineCache.mu.Unlock()

	if engine, ok := benchmarkEngineCache.data[key]; ok {
		return engine, nil
	}

	corpus := benchmarkCorpusFor(docs)
	opts := variant.opts
	if variant.withPrefixCache {
		opts = append(append([]Option(nil), opts...), WithPrefixCache(benchmarkPrefixes(corpus.items)))
	}
	engine, err := New(corpus.items, opts...)
	if err != nil {
		return nil, err
	}
	benchmarkEngineCache.data[key] = engine
	return engine, nil
}
```

Add `withPrefixCache` field to `benchmarkEngineVariant`:

```go
type benchmarkEngineVariant struct {
	name             string
	opts             []Option
	withPrefixCache  bool
}
```

- [ ] **Step 2: Add the cached_prefix benchmark case**

In `benchmark_search_test.go`, add to `benchmarkSearchCases` after the `prefix/default` entry:

```go
	{
		name:    "cached_prefix/default",
		variant: benchmarkEngineVariant{name: "cached", withPrefixCache: true},
		query:   func(c *benchmarkCorpus) string { return c.cachedPrefixQuery },
	},
```

- [ ] **Step 3: Run the benchmark to verify it works**

Run: `go test -run '^$' -bench 'BenchmarkSearch/cached_prefix' -benchmem -count=1`
Expected: ~12ns, 0 B/op, 0 allocs/op.

- [ ] **Step 4: Commit**

```bash
git add benchmark_search_test.go benchmark_fixture_test.go
git commit -m "bench: add cached_prefix benchmark case

Target: ~12ns/op, 0 allocs/op for prefix cache hits."
```

---

### Task 5: Add `WithPrefixCache` to `NewFromSnapshot` test

**Files:**
- Modify: `snapshot_test.go`

- [ ] **Step 1: Read the current snapshot test**

Read: `snapshot_test.go` to find the existing `TestSnapshotRoundTrip` or equivalent.

- [ ] **Step 2: Add a test verifying WithPrefixCache works with NewFromSnapshot**

Add to `snapshot_test.go`:

```go
func TestSnapshotWithPrefixCache(t *testing.T) {
	items := testItems()
	engine, err := New(items, WithBloom(100), WithFallbackField("category"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := engine.Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	// NewFromSnapshot with WithPrefixCache should succeed.
	restored, err := NewFromSnapshot(data, items, WithPrefixCache([]string{"b", "ni"}))
	if err != nil {
		t.Fatalf("NewFromSnapshot with WithPrefixCache: %v", err)
	}

	// Cached prefix should return results.
	results := restored.Search("b")
	if len(results) == 0 {
		t.Fatal("expected cached prefix results from restored engine")
	}

	// Non-cached query should still work normally.
	results = restored.Search("nike")
	if len(results) == 0 {
		t.Fatal("expected normal search results from restored engine")
	}
}
```

- [ ] **Step 3: Run test**

Run: `go test -run TestSnapshotWithPrefixCache -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add snapshot_test.go
git commit -m "test: verify WithPrefixCache works with NewFromSnapshot"
```

---

### Task 6: Add `ExtractPrefixes` helper to htmx-xsearch catalog

**Files:**
- Modify: `/home/mbow/code/search/htmx-xsearch/catalog/catalog.go`

- [ ] **Step 1: Write the failing test**

Create `/home/mbow/code/search/htmx-xsearch/catalog/catalog_test.go` (or add to existing):

```go
package catalog

import (
	"testing"
)

func TestExtractPrefixes(t *testing.T) {
	products := []Product{
		{Name: "Budweiser", Category: "beer"},
		{Name: "Bud Light", Category: "beer"},
		{Name: "Nike Air Max", Category: "shoes"},
	}

	prefixes := ExtractPrefixes(products)
	if len(prefixes) == 0 {
		t.Fatal("expected prefixes")
	}

	// Should contain lowercase 1 and 2 char prefixes.
	has := make(map[string]bool, len(prefixes))
	for _, p := range prefixes {
		has[p] = true
	}

	for _, want := range []string{"b", "bu", "n", "ni"} {
		if !has[want] {
			t.Errorf("missing expected prefix %q", want)
		}
	}

	// Should not contain duplicates.
	seen := make(map[string]struct{}, len(prefixes))
	for _, p := range prefixes {
		if _, ok := seen[p]; ok {
			t.Errorf("duplicate prefix %q", p)
		}
		seen[p] = struct{}{}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/mbow/code/search/htmx-xsearch && go test ./catalog/ -run TestExtractPrefixes -v`
Expected: FAIL — `ExtractPrefixes` not defined.

- [ ] **Step 3: Implement ExtractPrefixes**

Add to `/home/mbow/code/search/htmx-xsearch/catalog/catalog.go`:

```go
// ExtractPrefixes returns deduplicated 1-2 character prefixes from all
// product names, lowercased. Used to populate xsearch.WithPrefixCache.
func ExtractPrefixes(products []Product) []string {
	seen := make(map[string]struct{}, len(products)*2)
	for _, p := range products {
		name := strings.ToLower(p.Name)
		if len(name) >= 1 {
			seen[name[:1]] = struct{}{}
		}
		if len(name) >= 2 {
			seen[name[:2]] = struct{}{}
		}
	}
	prefixes := make([]string, 0, len(seen))
	for p := range seen {
		prefixes = append(prefixes, p)
	}
	return prefixes
}
```

- [ ] **Step 4: Run test**

Run: `cd /home/mbow/code/search/htmx-xsearch && go test ./catalog/ -run TestExtractPrefixes -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/mbow/code/search/htmx-xsearch
git add catalog/catalog.go catalog/catalog_test.go
git commit -m "feat: add ExtractPrefixes helper for xsearch prefix cache"
```

---

### Task 7: Wire `WithPrefixCache` into htmx-xsearch main.go

**Files:**
- Modify: `/home/mbow/code/search/htmx-xsearch/main.go`

- [ ] **Step 1: Add WithPrefixCache to engine construction**

In `main.go`, after loading products and snapshot, add prefix extraction and pass it to `NewFromSnapshot`:

Change:

```go
	eng, err := xsearch.NewFromSnapshot(snapshot, products,
		xsearch.WithLimit(10),
	)
```

To:

```go
	prefixes := catalog.ExtractPrefixes(products)
	eng, err := xsearch.NewFromSnapshot(snapshot, products,
		xsearch.WithLimit(10),
		xsearch.WithPrefixCache(prefixes),
	)
```

- [ ] **Step 2: Run all tests**

Run: `cd /home/mbow/code/search/htmx-xsearch && go test ./...`
Expected: All pass.

- [ ] **Step 3: Commit**

```bash
cd /home/mbow/code/search/htmx-xsearch
git add main.go
git commit -m "feat: enable prefix cache for autocomplete fast path

Extracts 1-2 char prefixes from product names and passes them
to xsearch.WithPrefixCache for O(1) prefix query responses."
```

---

### Task 8: Wire prefix cache into htmx-xsearch benchmarks

**Files:**
- Modify: `/home/mbow/code/search/htmx-xsearch/benchmarks/suite_test.go`

- [ ] **Step 1: Update `benchRuntime` to use prefix cache**

The `benchRuntime` helper builds the engine used by all benchmarks. Update it to match production:

```go
func benchRuntime(b *testing.B) (*xsearch.Engine, *ranking.Ranker) {
	b.Helper()
	products, err := catalog.LoadProducts("../data/products.json")
	if err != nil {
		b.Fatal(err)
	}
	snapshot, err := catalog.EmbeddedSnapshot()
	if err != nil {
		b.Fatal(err)
	}
	prefixes := catalog.ExtractPrefixes(products)
	ranker := ranking.New(0.05, 0.6)
	engine, err := xsearch.NewFromSnapshot(snapshot, products,
		xsearch.WithLimit(10),
		xsearch.WithPrefixCache(prefixes),
	)
	if err != nil {
		b.Fatal(err)
	}
	ranker.SetIDs(engine.IDs())
	return engine, ranker
}
```

- [ ] **Step 2: Run `BenchmarkEngine_Search_CachedPrefix` to verify**

Run: `cd /home/mbow/code/search/htmx-xsearch && go test -bench BenchmarkEngine_Search_CachedPrefix -benchmem -count=1 ./benchmarks/`
Expected: ~12ns, 0 allocs/op.

- [ ] **Step 3: Commit**

```bash
cd /home/mbow/code/search/htmx-xsearch
git add benchmarks/suite_test.go
git commit -m "bench: enable prefix cache in benchmark runtime"
```

---

### Task 9: Run full benchmarks, compare, and save results

**Files:**
- xsearch: `profiles/benchmarks/bench-latest.txt`
- htmx-xsearch: `docs/benchmarks/bench-latest.txt`

- [ ] **Step 1: Save current xsearch bench-latest as bench-prev**

```bash
cd /home/mbow/code/search/xsearch
make bench-save
```

- [ ] **Step 2: Record new xsearch benchmarks**

```bash
cd /home/mbow/code/search/xsearch
make bench-record
```

- [ ] **Step 3: Compare xsearch benchmarks**

```bash
cd /home/mbow/code/search/xsearch
make bench-compare
```

Expected: New `cached_prefix/default` cases at ~12ns. Other benchmarks unchanged.

- [ ] **Step 4: Save current htmx-xsearch bench-latest as bench-prev**

```bash
cd /home/mbow/code/search/htmx-xsearch
make bench-save
```

- [ ] **Step 5: Record new htmx-xsearch benchmarks**

```bash
cd /home/mbow/code/search/htmx-xsearch
make bench-record
```

- [ ] **Step 6: Compare htmx-xsearch benchmarks**

```bash
cd /home/mbow/code/search/htmx-xsearch
make bench-compare
```

Expected: `BenchmarkEngine_Search_CachedPrefix` drops from ~220ns to ~12ns. `BenchmarkHTTPServer_Search_ColdCache` may partially recover.

- [ ] **Step 7: Present benchmark results to user**

Print both benchstat outputs in full so the user can review the changes.

- [ ] **Step 8: Commit updated benchmark files**

```bash
cd /home/mbow/code/search/xsearch
git add profiles/benchmarks/
git commit -m "bench: record benchmarks with prefix cache"

cd /home/mbow/code/search/htmx-xsearch
git add docs/benchmarks/
git commit -m "bench: record benchmarks with prefix cache"
```

---

### Task 10: Update xsearch README

**Files:**
- Modify: `/home/mbow/code/search/xsearch/README.md`

- [ ] **Step 1: Add `WithPrefixCache` to the Configuration table**

In the Configuration section, add a new row after `WithFallbackField`:

```markdown
| `WithPrefixCache(prefixes)` | disabled | Precompute results for given queries |
```

- [ ] **Step 2: Add a Prefix Cache section after External Scoring**

Add a new section:

```markdown
## Prefix Cache

Precompute results for high-traffic queries (typically 1-2 character prefixes):

\`\`\`go
// Extract prefixes from your data.
var prefixes []string
for _, item := range items {
    name := strings.ToLower(item.Name)
    if len(name) >= 1 { prefixes = append(prefixes, name[:1]) }
    if len(name) >= 2 { prefixes = append(prefixes, name[:2]) }
}

engine, err := xsearch.New(items,
    xsearch.WithPrefixCache(prefixes),
)

// Cached queries return in ~12ns with zero allocations.
// When WithScoring is used, the cache is bypassed for fresh scored results.
results := engine.Search("b") // cache hit
\`\`\`
```

- [ ] **Step 3: Update benchmarks section**

Run `make bench-publish` to regenerate the benchmarks section from the latest data:

```bash
cd /home/mbow/code/search/xsearch
make bench-publish
```

This will update the `<!-- BENCHMARKS:START -->` / `<!-- BENCHMARKS:END -->` block.

- [ ] **Step 4: Commit**

```bash
cd /home/mbow/code/search/xsearch
git add README.md
git commit -m "docs: add prefix cache docs and update benchmarks"
```

---

### Task 11: Update htmx-xsearch README

**Files:**
- Modify: `/home/mbow/code/search/htmx-xsearch/README.md`

- [ ] **Step 1: Update the Architecture diagram to mention prefix cache**

In the Architecture section, update the `main.go` line:

```markdown
  main.go                              # Wires xsearch + server + ranking + prefix cache
```

- [ ] **Step 2: Update the "How It Works" flow diagram**

Add prefix cache to the flow:

```markdown
Query -> xsearch.Engine.Search()
           |
           +-> Normalize query
           +-> Prefix cache hit? (no scorer) -> return cached results
           |
           +-> Extract trigrams -> Bloom pre-check
```

- [ ] **Step 3: Update the Performance tables with fresh numbers**

Update the Engine Layer table with the cached prefix row. Replace the existing performance numbers with fresh benchstat output. Key change:

Add a row for cached prefix:

```markdown
| Cached prefix | `"b"` | **~12 ns** | 0 | 0 |
```

Update the Cold cache miss number if it improved.

- [ ] **Step 4: Add prefix cache to Key Optimisations**

Add a bullet:

```markdown
- **Prefix cache** — precomputed results for 1-2 char queries return in ~12ns with zero allocations; bypassed when external scoring is active
```

- [ ] **Step 5: Commit**

```bash
cd /home/mbow/code/search/htmx-xsearch
git add README.md
git commit -m "docs: update README with prefix cache and fresh benchmarks"
```
