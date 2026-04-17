# xsearch

Fuzzy search library for Go. Per-field BM25 and Jaccard trigram scoring with
optional bloom filter pre-rejection. Index any type. Handle typos, prefix
matching, and group fallback — all in-process.

```bash
go get github.com/mbow/xsearch
```

## Usage

Implement `Searchable` on your type:

```go
type Drink struct {
    ID          string
    Name        string
    Category    string
    Ingredients []string
    Tags        map[string][]string
}

func (d Drink) SearchID() string { return d.ID }

func (d Drink) SearchFields() []xsearch.Field {
    fields := []xsearch.Field{
        {Name: "name", Values: []string{d.Name}, Weight: 1.0},
        {Name: "category", Values: []string{d.Category}, Weight: 0.5},
    }
    if len(d.Ingredients) > 0 {
        fields = append(fields, xsearch.Field{
            Name: "ingredients", Values: d.Ingredients, Weight: 0.4,
        })
    }
    for key, vals := range d.Tags {
        fields = append(fields, xsearch.Field{
            Name: key, Values: vals, Weight: 0.3,
        })
    }
    return fields
}
```

Build and search:

```go
engine, err := xsearch.New(drinks,
    xsearch.WithBloom(100),
    xsearch.WithFallbackField("category"),
    xsearch.WithLimit(20),
)

results := engine.Search("smoky scotch")
for _, r := range results {
    item, _ := engine.Get(r.ID)
    fmt.Printf("%s (score: %.3f)\n", item.Fields[0].Values[0], r.Score)
}
```

## Configuration

| Option | Default | Purpose |
| ------ | ------: | ------- |
| `WithBloom(bitsPerItem)` | disabled | Bloom filter pre-rejection |
| `WithBM25(k1, b)` | 1.2, 0.75 | BM25 parameters |
| `WithAlpha(alpha)` | 0.6 | Relevance vs scorer blend weight |
| `WithLimit(n)` | 10 | Max results per search [2, 100] |
| `WithFallbackField(name)` | none | Field for group fallback |
| `WithPrefixCache(prefixes)` | disabled | Precompute results for given queries |
| `WithUnicodeFold()` | disabled | Accent-insensitive index/query normalization |

## External Scoring

Pass a `Scorer` per search to blend relevance with popularity or business logic:

```go
results := engine.Search("lager", xsearch.WithScoring(scorer))
```

The library normalizes scorer output per search (max-normalized to [0, 1]) and
blends with relevance using alpha. Negative values, NaN, and Inf are clamped
to zero.

## Prefix Cache

Precompute results for high-traffic queries (typically 1-2 character prefixes):

```go
// Extract prefixes from your data.
var prefixes []string
seen := make(map[string]struct{})
for _, item := range items {
    name := strings.ToLower(item.Name)
    if len(name) >= 1 {
        if _, ok := seen[name[:1]]; !ok { seen[name[:1]] = struct{}{}; prefixes = append(prefixes, name[:1]) }
    }
    if len(name) >= 2 {
        if _, ok := seen[name[:2]]; !ok { seen[name[:2]] = struct{}{}; prefixes = append(prefixes, name[:2]) }
    }
}

engine, err := xsearch.New(items,
    xsearch.WithPrefixCache(prefixes),
)

// Cached queries return in ~30ns with zero heap allocations.
// When WithScoring is used, the cache is bypassed for fresh scored results.
results := engine.Search("b") // cache hit
```

## Snapshots

Build indices once, serialize to CBOR, reload fast:

```go
data, _ := engine.Snapshot()
engine2, _ := xsearch.NewFromSnapshot(data, items)
```

Snapshots are self-contained CBOR blobs with a version header (`XSRC`).
Build-time index options are stored in the snapshot, including
`WithUnicodeFold()`. Load-time options `WithLimit`, `WithPrefixCache`, and
`WithScopes` may still be supplied on restore.

Snapshot version `2` breaks compatibility with older snapshot blobs. Rebuild
any existing snapshots before calling `NewFromSnapshot`.

## Bloom Filter

Use standalone or integrated via `WithBloom()`:

```go
bf := xsearch.NewBloom(10000, 100) // 10K items, 100 bits per item
bf.Add("trigram")
bf.MayContain("trigram") // true (no false negatives)
bf.MayContain("absent")  // false (rare false positives)
```

## Scoring Model

Each field is scored independently. Final relevance is the weighted sum:

**BM25 path** (primary): `relevance = sum(bm25(field_i, query) * field_i.Weight)`

**Jaccard fallback** (typos): `relevance = sum(jaccard(field_i_trigrams, query_trigrams) * field_i.Weight)`

Prefix boosting applies to the highest-weighted field only.

## Performance

These numbers are machine-dependent. Refresh this section with
`make bench-readme` after `make bench-record`, or use `make bench-publish`.

<!-- BENCHMARKS:START -->
_Generated from `profiles/benchmarks/bench-latest.txt` via `make bench-readme`. Metric cells are medians across the recorded samples._

- Commit: `09ff3d0`
- Branch: `main (dirty)`
- Date: `2026-04-09T19:16:04+01:00`
- Filter: `.`
- Count: `5`
- Time: `1s`
- CPU: `1`
- Package: `.`

### Build Engine

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `default/docs_128` | 4287987 | 3655642 | 25003 |
| `default/docs_2048` | 70130381 | 57548382 | 278383 |
| `default/docs_8192` | 287796248 | 231610736 | 1054363 |
| `bloom/docs_128` | 5159472 | 4041096 | 26861 |
| `bloom/docs_2048` | 81130343 | 63766896 | 308083 |
| `bloom/docs_8192` | 324255754 | 256528092 | 1173150 |
| `fallback/docs_128` | 4718848 | 3665760 | 25107 |
| `fallback/docs_2048` | 72041760 | 57589217 | 278519 |
| `fallback/docs_8192` | 294235486 | 231820540 | 1054523 |
| `full/docs_128` | 5211626 | 4051216 | 26965 |
| `full/docs_2048` | 81450853 | 63807739 | 308219 |
| `full/docs_8192` | 333087481 | 256737898 | 1173310 |

### Snapshot Encode

| Benchmark | ns/op | MB/s | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| `default/docs_128` | 907339 | 149.67 | 281539 | 26 |
| `default/docs_2048` | 9727357 | 251.48 | 4939687 | 33 |
| `default/docs_8192` | 38651944 | 256.26 | 19867137 | 35 |
| `full/docs_128` | 905951 | 151.86 | 281587 | 27 |
| `full/docs_2048` | 9967979 | 246.80 | 4972509 | 34 |
| `full/docs_8192` | 37905211 | 262.01 | 19920016 | 37 |

### Snapshot Load

| Benchmark | ns/op | MB/s | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| `default/docs_128` | 2878427 | 47.18 | 1315624 | 13860 |
| `default/docs_2048` | 40106643 | 60.99 | 18099395 | 132874 |
| `default/docs_8192` | 144709414 | 68.45 | 71957434 | 506549 |
| `full/docs_128` | 2927417 | 47.00 | 1327416 | 13969 |
| `full/docs_2048` | 40246276 | 61.13 | 18164939 | 133015 |
| `full/docs_8192` | 145007250 | 68.49 | 72265662 | 506714 |

### Component

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `bloom/add` | 19.16 | 0 | 0 |
| `bloom/may_contain_hit` | 14.84 | 0 | 0 |
| `bloom/may_contain_miss` | 5.614 | 0 | 0 |
| `extract_trigrams/allocating_medium` | 138.1 | 320 | 1 |
| `extract_trigrams/buffered_medium` | 13.31 | 0 | 0 |
| `extract_trigrams/buffered_long` | 37.06 | 0 | 0 |
| `tokenize/ascii` | 263.9 | 208 | 3 |
| `tokenize/mixed_case_unicode` | 457.1 | 248 | 8 |
| `bm25/docs_128` | 23847 | 2144 | 8 |
| `ngram/docs_128` | 9090 | 5088 | 7 |
| `bm25/docs_2048` | 368064 | 31320 | 12 |
| `ngram/docs_2048` | 19008 | 20016 | 10 |
| `fallback/exact/docs_128` | 11.87 | 0 | 0 |
| `fallback/best/docs_128` | 114.5 | 0 | 0 |
| `fallback/exact/docs_2048` | 11.90 | 0 | 0 |
| `fallback/best/docs_2048` | 115.6 | 0 | 0 |

### Search

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `exact/default/docs_128` | 37980 | 11952 | 71 |
| `exact/default/docs_2048` | 426382 | 87808 | 77 |
| `exact/default/docs_8192` | 1708830 | 329576 | 79 |
| `exact/bloom/docs_128` | 38355 | 11952 | 71 |
| `exact/bloom/docs_2048` | 418705 | 87808 | 77 |
| `exact/bloom/docs_8192` | 1752038 | 329576 | 79 |
| `prefix/default/docs_128` | 7228 | 7936 | 70 |
| `prefix/default/docs_2048` | 28572 | 25728 | 74 |
| `prefix/default/docs_8192` | 98797 | 88448 | 76 |
| `cached_prefix/default/docs_128` | 33.13 | 16 | 1 |
| `cached_prefix/default/docs_2048` | 33.68 | 16 | 1 |
| `cached_prefix/default/docs_8192` | 33.62 | 16 | 1 |
| `typo/default/docs_128` | 10373 | 5248 | 10 |
| `typo/default/docs_2048` | 21345 | 20256 | 13 |
| `typo/default/docs_8192` | 44944 | 9520 | 11 |
| `typo/bloom/docs_128` | 10251 | 5248 | 10 |
| `typo/bloom/docs_2048` | 21405 | 20256 | 13 |
| `typo/bloom/docs_8192` | 44771 | 9520 | 11 |
| `miss/default/docs_128` | 2149 | 16 | 1 |
| `miss/default/docs_2048` | 2164 | 16 | 1 |
| `miss/default/docs_8192` | 2127 | 16 | 1 |
| `miss_bloom/bloom/docs_128` | 362.1 | 16 | 1 |
| `miss_bloom/bloom/docs_2048` | 362.4 | 16 | 1 |
| `miss_bloom/bloom/docs_8192` | 361.9 | 16 | 1 |
| `fallback_exact/fallback/docs_128` | 5100 | 6680 | 46 |
| `fallback_exact/fallback/docs_2048` | 19665 | 27032 | 46 |
| `fallback_exact/fallback/docs_8192` | 70147 | 103616 | 48 |
| `fallback_typo/fallback/docs_128` | 6178 | 3456 | 12 |
| `fallback_typo/fallback/docs_2048` | 37913 | 24224 | 18 |
| `fallback_typo/fallback/docs_8192` | 37814 | 23712 | 18 |
| `scorer/default/docs_128` | 9288 | 8112 | 62 |
| `scorer/default/docs_2048` | 55055 | 30192 | 66 |
| `scorer/default/docs_8192` | 218168 | 106480 | 68 |
| `limit_10/default/docs_128` | 8373 | 7496 | 59 |
| `limit_10/default/docs_2048` | 47946 | 25288 | 63 |
| `limit_10/default/docs_8192` | 184764 | 88008 | 65 |
| `limit_100/wide/docs_128` | 11056 | 10808 | 89 |
| `limit_100/wide/docs_2048` | 101712 | 74328 | 513 |
| `limit_100/wide/docs_8192` | 307146 | 137048 | 515 |
| `multiword/default/docs_128` | 12828 | 7848 | 58 |
| `multiword/default/docs_2048` | 75095 | 25288 | 57 |
| `multiword/default/docs_8192` | 318194 | 90536 | 65 |

### Parallel Search

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `exact/default/docs_8192` | 1772159 | 329576 | 79 |
| `typo/default/docs_8192` | 46229 | 9520 | 11 |
| `miss/default/docs_8192` | 2126 | 16 | 1 |
| `miss_bloom/bloom/docs_8192` | 364.4 | 16 | 1 |
<!-- BENCHMARKS:END -->

## License

MIT

## Changelog

### v0.2.0

- Snapshot format bumped to version `2`; rebuild any existing snapshot blobs
  before calling `NewFromSnapshot`.
- `WithUnicodeFold()` is now serialized into snapshots and restored on load, so
  folded engines round-trip without behavior drift.
- `NewFromSnapshot` now rejects `WithUnicodeFold()` as a load-time override
  because it is part of the persisted snapshot config.

### v0.1.0

- `Fold(s string) string` — exported Unicode NFKD normalization with combining-mark
  stripping and ligature expansion (`œ→oe`, `æ→ae`, `ß→ss`). Idempotent.
- `WithUnicodeFold() Option` — index- and query-time accent-insensitive search
  using `Fold`. Integrated with `WithPrefixCache` so prewarm keys hit after
  folding+normalization.
- `Engine.SearchWithFallback(primary, cascade, opts ...SearchOption) ([]Result, int)` —
  runs `primary`; if empty, walks `cascade` in order and returns the first
  non-empty level. Level `-1` = primary matched; `0..len(cascade)-1` = cascade
  index; `len(cascade)` = no match.
- `WithFilter(pred func(id string) bool) SearchOption` — per-search predicate
  applied after scope filtering and before scoring/limit. Prefix-cache fast
  path is bypassed when a filter is set. Nil filter is zero-overhead.
