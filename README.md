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

- Commit: `28bda81`
- Branch: `main (dirty)`
- Date: `2026-04-09T16:28:20+01:00`
- Filter: `^Benchmark`
- Count: `3`
- Time: `100ms`
- CPU: `1`
- Package: `.`

### Build Engine

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `default/docs_128` | 4576235 | 3655290 | 24995 |
| `default/docs_2048` | 76754792 | 57548048 | 278375 |
| `default/docs_8192` | 309017926 | 231610392 | 1054355 |
| `bloom/docs_128` | 5554090 | 4040744 | 26853 |
| `bloom/docs_2048` | 88319130 | 63766544 | 308075 |
| `bloom/docs_8192` | 352020980 | 256527752 | 1173142 |
| `fallback/docs_128` | 4984208 | 3665408 | 25099 |
| `fallback/docs_2048` | 78635950 | 57588864 | 278511 |
| `fallback/docs_8192` | 305023837 | 231820192 | 1054515 |
| `full/docs_128` | 5512208 | 4050864 | 26957 |
| `full/docs_2048` | 87880552 | 63807392 | 308211 |
| `full/docs_8192` | 355866777 | 256737536 | 1173302 |

### Snapshot Encode

| Benchmark | ns/op | MB/s | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| `default/docs_128` | 928303 | 146.29 | 305469 | 30 |
| `default/docs_2048` | 11302960 | 216.43 | 4939900 | 35 |
| `default/docs_8192` | 42493845 | 233.10 | 19894018 | 47 |
| `full/docs_128` | 952087 | 144.50 | 303711 | 31 |
| `full/docs_2048` | 10953426 | 224.60 | 4972646 | 35 |
| `full/docs_8192` | 41143136 | 241.39 | 19947856 | 59 |

### Snapshot Load

| Benchmark | ns/op | MB/s | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| `default/docs_128` | 3022126 | 44.94 | 1315416 | 13858 |
| `default/docs_2048` | 42967470 | 56.93 | 18099186 | 132872 |
| `default/docs_8192` | 148882528 | 66.53 | 71957224 | 506547 |
| `full/docs_128` | 3095968 | 44.44 | 1327208 | 13967 |
| `full/docs_2048` | 41679281 | 59.02 | 18164728 | 133013 |
| `full/docs_8192` | 151369912 | 65.61 | 72265448 | 506712 |

### Component

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `bloom/add` | 19.00 | 0 | 0 |
| `bloom/may_contain_hit` | 15.03 | 0 | 0 |
| `bloom/may_contain_miss` | 6.384 | 0 | 0 |
| `extract_trigrams/allocating_medium` | 137.5 | 320 | 1 |
| `extract_trigrams/buffered_medium` | 13.68 | 0 | 0 |
| `extract_trigrams/buffered_long` | 37.65 | 0 | 0 |
| `tokenize/ascii` | 270.3 | 208 | 3 |
| `tokenize/mixed_case_unicode` | 465.9 | 248 | 8 |
| `bm25/docs_128` | 29613 | 2336 | 10 |
| `ngram/docs_128` | 9464 | 5088 | 7 |
| `bm25/docs_2048` | 423640 | 31512 | 14 |
| `ngram/docs_2048` | 18997 | 20016 | 10 |
| `fallback/exact/docs_128` | 12.08 | 0 | 0 |
| `fallback/best/docs_128` | 121.0 | 0 | 0 |
| `fallback/exact/docs_2048` | 12.24 | 0 | 0 |
| `fallback/best/docs_2048` | 119.3 | 0 | 0 |

### Search

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `exact/default/docs_128` | 44499 | 14320 | 77 |
| `exact/default/docs_2048` | 535314 | 131009 | 91 |
| `exact/default/docs_8192` | 1998356 | 520354 | 101 |
| `exact/bloom/docs_128` | 42744 | 14320 | 77 |
| `exact/bloom/docs_2048` | 496903 | 131008 | 91 |
| `exact/bloom/docs_8192` | 2159095 | 520354 | 101 |
| `prefix/default/docs_128` | 9620 | 8688 | 73 |
| `prefix/default/docs_2048` | 47724 | 36048 | 83 |
| `prefix/default/docs_8192` | 173056 | 131472 | 89 |
| `typo/default/docs_128` | 9084 | 5440 | 12 |
| `typo/default/docs_2048` | 21655 | 20448 | 15 |
| `typo/default/docs_8192` | 35474 | 9712 | 13 |
| `typo/bloom/docs_128` | 9271 | 5440 | 12 |
| `typo/bloom/docs_2048` | 21971 | 20448 | 15 |
| `typo/bloom/docs_8192` | 36372 | 9712 | 13 |
| `miss/default/docs_128` | 2445 | 80 | 2 |
| `miss/default/docs_2048` | 2540 | 80 | 2 |
| `miss/default/docs_8192` | 2508 | 80 | 2 |
| `miss_bloom/bloom/docs_128` | 373.3 | 16 | 1 |
| `miss_bloom/bloom/docs_2048` | 370.6 | 16 | 1 |
| `miss_bloom/bloom/docs_8192` | 361.2 | 16 | 1 |
| `fallback_exact/fallback/docs_128` | 5657 | 6680 | 46 |
| `fallback_exact/fallback/docs_2048` | 20211 | 27032 | 46 |
| `fallback_exact/fallback/docs_8192` | 66036 | 103616 | 48 |
| `fallback_typo/fallback/docs_128` | 6028 | 4208 | 15 |
| `fallback_typo/fallback/docs_2048` | 40529 | 34544 | 27 |
| `fallback_typo/fallback/docs_8192` | 39534 | 34032 | 27 |
| `scorer/default/docs_128` | 9275 | 8864 | 65 |
| `scorer/default/docs_2048` | 64141 | 40512 | 75 |
| `scorer/default/docs_8192` | 252773 | 149504 | 81 |
| `limit_10/default/docs_128` | 8359 | 8248 | 62 |
| `limit_10/default/docs_2048` | 57840 | 35608 | 72 |
| `limit_10/default/docs_8192` | 231125 | 131032 | 78 |
| `limit_100/wide/docs_128` | 10358 | 11560 | 92 |
| `limit_100/wide/docs_2048` | 103868 | 84648 | 522 |
| `limit_100/wide/docs_8192` | 344872 | 180072 | 528 |
| `multiword/default/docs_128` | 13453 | 8616 | 61 |
| `multiword/default/docs_2048` | 96400 | 35624 | 66 |
| `multiword/default/docs_8192` | 381849 | 133576 | 78 |

### Parallel Search

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `exact/default/docs_8192` | 2011297 | 520357 | 101 |
| `typo/default/docs_8192` | 37353 | 9712 | 13 |
| `miss/default/docs_8192` | 2479 | 80 | 2 |
| `miss_bloom/bloom/docs_8192` | 371.6 | 16 | 1 |
<!-- BENCHMARKS:END -->

## License

MIT
