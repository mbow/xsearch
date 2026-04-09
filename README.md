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

## External Scoring

Pass a `Scorer` per search to blend relevance with popularity or business logic:

```go
results := engine.Search("lager", xsearch.WithScoring(scorer))
```

The library normalizes scorer output per search (max-normalized to [0, 1]) and
blends with relevance using alpha. Negative values, NaN, and Inf are clamped
to zero.

## Snapshots

Build indices once, serialize to CBOR, reload fast:

```go
data, _ := engine.Snapshot()
engine2, _ := xsearch.NewFromSnapshot(data, items)
```

Snapshots are self-contained CBOR blobs with a version header (`XSRC`).
Build-time options are stored in the snapshot. Only `WithLimit` can be
overridden at load time.

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

- Commit: `2e15419`
- Branch: `main (dirty)`
- Date: `2026-04-09T16:45:30+01:00`
- Filter: `^Benchmark`
- Count: `3`
- Time: `100ms`
- CPU: `1`
- Package: `.`

### Build Engine

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `default/docs_128` | 4494809 | 3655594 | 25003 |
| `default/docs_2048` | 77177376 | 57548336 | 278383 |
| `default/docs_8192` | 322137476 | 231610712 | 1054363 |
| `bloom/docs_128` | 5478499 | 4041048 | 26861 |
| `bloom/docs_2048` | 88669916 | 63766856 | 308083 |
| `bloom/docs_8192` | 351805438 | 256528040 | 1173150 |
| `fallback/docs_128` | 5196454 | 3665712 | 25107 |
| `fallback/docs_2048` | 77168720 | 57589168 | 278519 |
| `fallback/docs_8192` | 306155469 | 231820480 | 1054523 |
| `full/docs_128` | 5608596 | 4051168 | 26965 |
| `full/docs_2048` | 87649232 | 63807696 | 308219 |
| `full/docs_8192` | 352762781 | 256737840 | 1173310 |

### Snapshot Encode

| Benchmark | ns/op | MB/s | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| `default/docs_128` | 953422 | 142.44 | 321264 | 34 |
| `default/docs_2048` | 10975199 | 222.89 | 4939841 | 35 |
| `default/docs_8192` | 41514521 | 238.59 | 19894912 | 53 |
| `full/docs_128` | 969484 | 141.91 | 281605 | 27 |
| `full/docs_2048` | 11004279 | 223.56 | 4977077 | 37 |
| `full/docs_8192` | 41345153 | 240.21 | 19943298 | 50 |

### Snapshot Load

| Benchmark | ns/op | MB/s | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| `default/docs_128` | 3040189 | 44.67 | 1315576 | 13860 |
| `default/docs_2048` | 40433985 | 60.50 | 18099346 | 132874 |
| `default/docs_8192` | 146434385 | 67.64 | 71957384 | 506549 |
| `full/docs_128` | 3112726 | 44.20 | 1327368 | 13969 |
| `full/docs_2048` | 41701350 | 58.99 | 18164893 | 133015 |
| `full/docs_8192` | 144527378 | 68.72 | 72265608 | 506714 |

### Component

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `bloom/add` | 18.99 | 0 | 0 |
| `bloom/may_contain_hit` | 15.05 | 0 | 0 |
| `bloom/may_contain_miss` | 6.002 | 0 | 0 |
| `extract_trigrams/allocating_medium` | 144.7 | 320 | 1 |
| `extract_trigrams/buffered_medium` | 13.60 | 0 | 0 |
| `extract_trigrams/buffered_long` | 37.69 | 0 | 0 |
| `tokenize/ascii` | 275.9 | 208 | 3 |
| `tokenize/mixed_case_unicode` | 468.1 | 248 | 8 |
| `bm25/docs_128` | 25112 | 2144 | 8 |
| `ngram/docs_128` | 9455 | 5088 | 7 |
| `bm25/docs_2048` | 386813 | 31320 | 12 |
| `ngram/docs_2048` | 19010 | 20016 | 10 |
| `fallback/exact/docs_128` | 11.99 | 0 | 0 |
| `fallback/best/docs_128` | 128.7 | 0 | 0 |
| `fallback/exact/docs_2048` | 12.23 | 0 | 0 |
| `fallback/best/docs_2048` | 117.9 | 0 | 0 |

### Search

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `exact/default/docs_128` | 41148 | 11952 | 71 |
| `exact/default/docs_2048` | 452415 | 87808 | 77 |
| `exact/default/docs_8192` | 1813745 | 329578 | 79 |
| `exact/bloom/docs_128` | 35947 | 11952 | 71 |
| `exact/bloom/docs_2048` | 424352 | 87808 | 77 |
| `exact/bloom/docs_8192` | 1833319 | 329578 | 79 |
| `prefix/default/docs_128` | 7060 | 7936 | 70 |
| `prefix/default/docs_2048` | 28567 | 25728 | 74 |
| `prefix/default/docs_8192` | 103516 | 88448 | 76 |
| `typo/default/docs_128` | 8591 | 5248 | 10 |
| `typo/default/docs_2048` | 20979 | 20256 | 13 |
| `typo/default/docs_8192` | 35531 | 9520 | 11 |
| `typo/bloom/docs_128` | 8927 | 5248 | 10 |
| `typo/bloom/docs_2048` | 21410 | 20256 | 13 |
| `typo/bloom/docs_8192` | 36681 | 9520 | 11 |
| `miss/default/docs_128` | 2210 | 16 | 1 |
| `miss/default/docs_2048` | 2221 | 16 | 1 |
| `miss/default/docs_8192` | 2277 | 16 | 1 |
| `miss_bloom/bloom/docs_128` | 365.6 | 16 | 1 |
| `miss_bloom/bloom/docs_2048` | 363.3 | 16 | 1 |
| `miss_bloom/bloom/docs_8192` | 368.1 | 16 | 1 |
| `fallback_exact/fallback/docs_128` | 5768 | 6680 | 46 |
| `fallback_exact/fallback/docs_2048` | 21150 | 27032 | 46 |
| `fallback_exact/fallback/docs_8192` | 65802 | 103616 | 48 |
| `fallback_typo/fallback/docs_128` | 5393 | 3456 | 12 |
| `fallback_typo/fallback/docs_2048` | 31905 | 24224 | 18 |
| `fallback_typo/fallback/docs_8192` | 31005 | 23712 | 18 |
| `scorer/default/docs_128` | 9424 | 8112 | 62 |
| `scorer/default/docs_2048` | 49113 | 30192 | 66 |
| `scorer/default/docs_8192` | 197358 | 106480 | 68 |
| `limit_10/default/docs_128` | 8806 | 7496 | 59 |
| `limit_10/default/docs_2048` | 42113 | 25288 | 63 |
| `limit_10/default/docs_8192` | 168879 | 88008 | 65 |
| `limit_100/wide/docs_128` | 10437 | 10808 | 89 |
| `limit_100/wide/docs_2048` | 85380 | 74328 | 513 |
| `limit_100/wide/docs_8192` | 264122 | 137048 | 515 |
| `multiword/default/docs_128` | 11214 | 7848 | 58 |
| `multiword/default/docs_2048` | 75398 | 25288 | 57 |
| `multiword/default/docs_8192` | 306857 | 90536 | 65 |

### Parallel Search

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `exact/default/docs_8192` | 1862987 | 329580 | 79 |
| `typo/default/docs_8192` | 36352 | 9520 | 11 |
| `miss/default/docs_8192` | 2267 | 16 | 1 |
| `miss_bloom/bloom/docs_8192` | 363.0 | 16 | 1 |
<!-- BENCHMARKS:END -->

## License

MIT
