# Prefix Result Cache for xsearch

## Problem

When xsearch was extracted from the htmx-xsearch monolith into a standalone library (v0.0.2), two internal caches were dropped:

1. **Prefix cache** — precomputed results for all 1-2 character query prefixes (~12ns lookups, zero allocations)
2. **Category cache** — precomputed top-N products per category with dirty-tracking for popularity changes

The prefix cache removal caused an **87% regression** on the cold-cache HTTP search path (6.7µs to 12.5µs) and nearly doubled allocations (38 to 73 per request). Short-prefix queries are the highest-traffic queries in an autocomplete system since every search starts with 1-2 characters.

The category cache is inherently tied to mutable ranking state and belongs in the application layer. This spec covers only the prefix cache, which is deterministic and stateless.

## Decision

Restore prefix caching as an opt-in construction option in the xsearch library. The consumer precomputes the prefix list from their data and passes it at engine construction time. The engine builds results for each prefix during `New()` / `NewFromSnapshot()` and serves them directly on cache hits.

### Alternatives considered

- **Post-construction `BuildPrefixCache()` method**: Breaks the engine's immutable-after-construction invariant. Requires the consumer to remember a second init step. Every construction path (`New`, `NewFromSnapshot`) needs the follow-up call.
- **Generic query result cache (`WithResultCache(map[string][]Result)`)**: Chicken-and-egg problem — you need the engine to build results, but the engine needs the cache. Would require building a throwaway engine first.

## Design

### Public API

One new option function and one new config option:

```go
// WithPrefixCache precomputes search results for the given queries during
// engine construction. Cached queries return in O(1) with zero allocations.
// Typical usage: pass all 1-2 character prefixes of the primary search field.
func WithPrefixCache(prefixes []string) Option
```

Usage:

```go
engine, err := xsearch.New(products,
    xsearch.WithBloom(100),
    xsearch.WithPrefixCache(prefixes),
)
```

Works identically with `NewFromSnapshot()`.

### Engine internals

#### New field

```go
type Engine struct {
    // ... existing fields ...
    prefixCache map[string][]Result
}
```

#### Construction

At the end of `newEngineFromPrepared()`, after all indexes (bloom, bm25, ngram, fallback) are built:

1. If `cfg.prefixCacheKeys` is nil or empty, skip — no cache.
2. Normalize each prefix (same `normalizeQuery` used by `Search`).
3. Deduplicate the prefix list.
4. For each prefix, run the internal search pipeline (the same path `Search()` uses, but bypassing the cache check) and store the results.
5. Assign the populated map to `e.prefixCache`.

#### Search fast path

At the top of `Engine.Search()`, before any other work:

```go
if e.prefixCache != nil {
    if cached, ok := e.prefixCache[query]; ok {
        return cached
    }
}
```

This check happens after `normalizeQuery` but before trigram extraction, bloom checks, or scoring. A cache hit returns immediately — zero allocations, no index traversal.

#### Scorer interaction

When `WithScoring()` is passed to a `Search()` call that hits the prefix cache, the cache is bypassed and the full search pipeline runs. This ensures popularity-weighted results are always fresh. The check becomes:

```go
if e.prefixCache != nil && sCfg.scorer == nil {
    if cached, ok := e.prefixCache[query]; ok {
        return cached
    }
}
```

This means:
- Bare `engine.Search("b")` → cache hit, ~12ns
- `engine.Search("b", WithScoring(scorer))` → cache miss, full pipeline with scoring

The app layer's `FragmentCache` handles caching of scored results.

### Snapshot behavior

The prefix cache is **not** serialized into snapshots. It is rebuilt from the `WithPrefixCache` option on every construction. Rationale:

- Snapshots stay lean (the cache for ~800 prefixes would add negligible size, but it's redundant data).
- The consumer already passes the option — rebuild is automatic.
- Build cost is low (~2ms for 800 prefixes on the benchmark hardware).

`WithPrefixCache` is allowed with `NewFromSnapshot` (it does not fail `validateForSnapshotLoad`).

### What the consumer does (htmx-xsearch)

A new helper in the `catalog` package extracts prefixes from product data:

```go
// ExtractPrefixes returns deduplicated 1-2 character prefixes from all
// product names, lowercased. Used to populate xsearch.WithPrefixCache.
func ExtractPrefixes(products []catalog.Product) []string
```

Usage in `main.go`:

```go
prefixes := catalog.ExtractPrefixes(products)
engine, err := xsearch.NewFromSnapshot(snapshot, products,
    xsearch.WithLimit(10),
    xsearch.WithPrefixCache(prefixes),
)
```

## Testing

### xsearch lib

- **Unit test**: `TestPrefixCacheHit` — verify that `Search(prefix)` with cache returns identical results to `Search(prefix)` without cache (no scorer in either case).
- **Unit test**: `TestPrefixCacheBypassWithScorer` — verify that `Search(prefix, WithScoring(scorer))` does NOT use the cache and returns scorer-weighted results.
- **Unit test**: `TestPrefixCacheNormalization` — verify that mixed-case prefixes are normalized and deduplicated.
- **Benchmark**: `BenchmarkSearch` sub-benchmarks should include a cached-prefix case. Target: ~12ns, 0 allocs/op.

### htmx-xsearch

- **Benchmark**: `BenchmarkEngine_Search_CachedPrefix` should return to ~12ns from whatever the current uncached time is.
- **Integration**: Verify the `ExtractPrefixes` helper produces the expected set for the test product data.

## Performance expectations

| Path | Before (v0.0.1) | Current (v0.0.2) | After this change |
|------|-----------------|-------------------|-------------------|
| Cached prefix (no scorer) | ~12ns, 0 allocs | ~220ns, 0 allocs | ~12ns, 0 allocs |
| Cold cache HTTP (prefix query) | ~6.7µs, 38 allocs | ~12.5µs, 73 allocs | Partial recovery (render-path allocs remain) |
| Warm cache HTTP | ~2.4µs, 24 allocs | ~2.4µs, 24 allocs | ~2.4µs, 24 allocs |

The cold-cache HTTP regression is partly caused by the prefix cache loss and partly by render-path changes (lookup-per-result vs pre-attached product pointers). This spec addresses the prefix cache; render-path optimization is a separate concern.
