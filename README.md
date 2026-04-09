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

## License

MIT
