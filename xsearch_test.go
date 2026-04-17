package xsearch

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

type testItem struct {
	id     string
	fields []Field
}

func (t testItem) SearchID() string      { return t.id }
func (t testItem) SearchFields() []Field { return t.fields }

type mapScorer map[int]float64

func (m mapScorer) Score(docIndex int) float64 { return m[docIndex] }

func testItems() []testItem {
	return []testItem{
		{id: "budweiser", fields: []Field{{Name: "name", Values: []string{"Budweiser"}, Weight: 1.0}, {Name: "category", Values: []string{"beer"}, Weight: 0.5}}},
		{id: "bud-light", fields: []Field{{Name: "name", Values: []string{"Bud Light"}, Weight: 1.0}, {Name: "category", Values: []string{"beer"}, Weight: 0.5}}},
		{id: "funky-buddha", fields: []Field{{Name: "name", Values: []string{"Funky Buddha Double Lambic"}, Weight: 1.0}, {Name: "category", Values: []string{"beer"}, Weight: 0.5}}},
		{id: "nike-air-max", fields: []Field{{Name: "name", Values: []string{"Nike Air Max"}, Weight: 1.0}, {Name: "category", Values: []string{"shoes"}, Weight: 0.5}}},
		{id: "nike-dunk", fields: []Field{{Name: "name", Values: []string{"Nike Dunk Low"}, Weight: 1.0}, {Name: "category", Values: []string{"shoes"}, Weight: 0.5}}},
		{id: "smoky-scotch", fields: []Field{{Name: "name", Values: []string{"Smoky Scotch"}, Weight: 1.0}, {Name: "category", Values: []string{"spirits"}, Weight: 0.5}, {Name: "tags", Values: []string{"peaty", "smoky"}, Weight: 0.4}}},
	}
}

func TestNewRejectsDuplicateIDs(t *testing.T) {
	_, err := New([]testItem{
		{id: "a", fields: []Field{{Name: "name", Values: []string{"alpha"}, Weight: 1}}},
		{id: "a", fields: []Field{{Name: "name", Values: []string{"beta"}, Weight: 1}}},
	})
	if err == nil {
		t.Fatal("expected error for duplicate IDs")
	}
}

func TestNewRejectsInvalidFields(t *testing.T) {
	_, err := New([]testItem{{id: "a", fields: []Field{{Name: "", Values: []string{"alpha"}, Weight: 1}}}})
	if err == nil {
		t.Fatal("expected error for empty field name")
	}

	_, err = New([]testItem{{id: "a", fields: []Field{{Name: "name", Values: []string{"alpha"}, Weight: 0}}}})
	if err == nil {
		t.Fatal("expected error for non-positive weight")
	}

	_, err = New([]testItem{{id: "a", fields: []Field{{Name: "name", Values: []string{"alpha"}, Weight: 1}, {Name: "name", Values: []string{"beta"}, Weight: 1}}}})
	if err == nil {
		t.Fatal("expected error for duplicate field names")
	}
}

func TestWithLimitValidation(t *testing.T) {
	items := []testItem{{id: "a", fields: []Field{{Name: "name", Values: []string{"alpha"}, Weight: 1}}}}
	if _, err := New(items, WithLimit(1)); err == nil {
		t.Fatal("expected error for limit < 2")
	}
	if _, err := New(items, WithLimit(101)); err == nil {
		t.Fatal("expected error for limit > 100")
	}
}

func TestSearchDirectMatch(t *testing.T) {
	engine, err := New(testItems())
	if err != nil {
		t.Fatal(err)
	}
	results := engine.Search("nike")
	if len(results) == 0 {
		t.Fatal("expected results for nike")
	}
	if results[0].MatchType != MatchDirect {
		t.Fatalf("expected direct result, got %v", results[0].MatchType)
	}
}

func TestSearchFuzzyMatch(t *testing.T) {
	engine, err := New(testItems())
	if err != nil {
		t.Fatal(err)
	}
	results := engine.Search("budwiser")
	if len(results) == 0 || results[0].ID != "budweiser" {
		t.Fatalf("expected budweiser result, got %+v", results)
	}
}

func TestSearchFallbackExactGroup(t *testing.T) {
	engine, err := New(testItems(), WithFallbackField("category"))
	if err != nil {
		t.Fatal(err)
	}
	results := engine.Search("beer")
	if len(results) == 0 {
		t.Fatal("expected fallback results")
	}
	for _, result := range results {
		if result.MatchType != MatchFallback {
			t.Fatalf("expected fallback result, got %v", result.MatchType)
		}
	}
}

func TestSearchSortedByScore(t *testing.T) {
	engine, err := New(testItems())
	if err != nil {
		t.Fatal(err)
	}
	results := engine.Search("nike", WithScoring(mapScorer{4: 10}))
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Fatalf("results not sorted at %d", i)
		}
	}
}

func TestMultiValueHighlightValueIndex(t *testing.T) {
	engine, err := New(testItems())
	if err != nil {
		t.Fatal(err)
	}
	results := engine.Search("smoky")
	if len(results) == 0 {
		t.Fatal("expected smoky result")
	}
	highlights := results[0].Highlights["tags"]
	if len(highlights) == 0 {
		t.Fatal("expected tag highlight")
	}
	if highlights[0].ValueIndex != 1 {
		t.Fatalf("expected value index 1, got %d", highlights[0].ValueIndex)
	}
}

func TestGetAndIDs(t *testing.T) {
	engine, err := New(testItems())
	if err != nil {
		t.Fatal(err)
	}
	if ids := engine.IDs(); len(ids) != len(testItems()) || ids[0] != "budweiser" {
		t.Fatalf("unexpected ids: %v", ids)
	}
	item, ok := engine.Get("nike-air-max")
	if !ok {
		t.Fatal("expected item lookup")
	}
	if item.ID != "nike-air-max" || item.Fields[0].Values[0] != "Nike Air Max" {
		t.Fatalf("unexpected item: %+v", item)
	}
	item.Fields[0].Values[0] = "mutated"
	item2, _ := engine.Get("nike-air-max")
	if item2.Fields[0].Values[0] != "Nike Air Max" {
		t.Fatal("Get should return a copy")
	}
}

func TestScorerClamping(t *testing.T) {
	engine, err := New(testItems())
	if err != nil {
		t.Fatal(err)
	}
	results := engine.Search("bud", WithScoring(mapScorer{
		0: -1,
		1: math.Inf(1),
		2: math.NaN(),
	}))
	if len(results) == 0 {
		t.Fatal("expected results")
	}
}

func TestSearchWithBloom(t *testing.T) {
	eng, err := New(testItems(), WithBloom(100))
	if err != nil {
		t.Fatal(err)
	}
	results := eng.Search("budweiser")
	if len(results) == 0 {
		t.Fatal("expected results with bloom enabled")
	}
	if results[0].ID != "budweiser" {
		t.Fatalf("expected budweiser, got %q", results[0].ID)
	}
}

func TestSearchBloomRejectsNonsense(t *testing.T) {
	eng, err := New(testItems(), WithBloom(100))
	if err != nil {
		t.Fatal(err)
	}
	results := eng.Search("xzqwvp")
	if len(results) != 0 {
		t.Fatalf("expected bloom to reject nonsense, got %d results", len(results))
	}
}

func TestSearchWithoutBloomMatchesWithBloom(t *testing.T) {
	items := testItems()
	engNo, _ := New(items)
	engYes, _ := New(items, WithBloom(100))

	queries := []string{"budweiser", "nike", "bud"}
	for _, q := range queries {
		r1 := engNo.Search(q)
		r2 := engYes.Search(q)
		if len(r1) != len(r2) {
			t.Fatalf("query %q: result count differs: %d vs %d (bloom)", q, len(r1), len(r2))
		}
		for i := range r1 {
			if r1[i].ID != r2[i].ID {
				t.Fatalf("query %q result %d: ID %q vs %q (bloom)", q, i, r1[i].ID, r2[i].ID)
			}
		}
	}
}

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

func TestPrefixCacheNormalization(t *testing.T) {
	items := testItems()
	engine, err := New(items, WithPrefixCache([]string{"B", "b", "BU", "bu", " b "}))
	if err != nil {
		t.Fatal(err)
	}

	r1 := engine.Search("b")
	r2 := engine.Search("B")
	if len(r1) == 0 {
		t.Fatal("expected results for 'b'")
	}
	if len(r1) != len(r2) {
		t.Fatalf("normalized queries should return same results: %d vs %d", len(r1), len(r2))
	}
}

func TestPrefixCacheBypassWithScorer(t *testing.T) {
	items := testItems()
	engine, err := New(items, WithPrefixCache([]string{"bud"}))
	if err != nil {
		t.Fatal(err)
	}

	uncached, _ := New(items)
	cachedResults := engine.Search("bud")
	uncachedResults := uncached.Search("bud")
	if len(cachedResults) != len(uncachedResults) {
		t.Fatalf("cached count %d != uncached count %d", len(cachedResults), len(uncachedResults))
	}

	scorer := mapScorer{0: 100, 1: 0}
	scoredResults := engine.Search("bud", WithScoring(scorer))
	if len(scoredResults) == 0 {
		t.Fatal("expected scored results")
	}
	if len(scoredResults) == len(cachedResults) && scoredResults[0].Score == cachedResults[0].Score {
		t.Fatal("scorer should produce different scores than cached results")
	}
}

func TestWithScopesRejectsUnknownIDs(t *testing.T) {
	_, err := New(testItems(), WithScopes(map[string][]string{
		"beer": {"budweiser", "missing-id"},
	}))
	if err == nil {
		t.Fatal("expected error for scope containing unknown ID")
	}
}

func TestSearchWithScope(t *testing.T) {
	engine, err := New(testItems(), WithScopes(map[string][]string{
		"beer":  {"budweiser", "bud-light", "funky-buddha"},
		"shoes": {"nike-air-max", "nike-dunk"},
	}))
	if err != nil {
		t.Fatal(err)
	}

	results := engine.Search("nike", WithScope("shoes"))
	if len(results) == 0 {
		t.Fatal("expected scoped nike results")
	}
	for _, result := range results {
		if result.ID != "nike-air-max" && result.ID != "nike-dunk" {
			t.Fatalf("unexpected scoped result %q", result.ID)
		}
	}

	results = engine.Search("nike", WithScope("beer"))
	if len(results) != 0 {
		t.Fatalf("expected no beer-scoped nike results, got %+v", results)
	}
}

func TestNewFromSnapshotRejectsBuildOptions(t *testing.T) {
	items := testItems()
	engine, err := New(items)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := engine.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewFromSnapshot(snapshot, items, WithBloom(10)); err == nil {
		t.Fatal("expected build-time option rejection")
	}
}

type accentItem struct {
	id, name string
}

func (a accentItem) SearchID() string { return a.id }
func (a accentItem) SearchFields() []Field {
	return []Field{{Name: "name", Values: []string{a.name}, Weight: 1.0}}
}

func TestWithUnicodeFold_OffByDefault_DistinctTokens(t *testing.T) {
	items := []accentItem{{id: "moet", name: "Moët & Chandon"}}
	e, err := New(items)
	if err != nil {
		t.Fatal(err)
	}
	if r := e.Search("moet"); len(r) != 0 {
		t.Errorf("expected zero results without fold, got %d", len(r))
	}
}

func TestWithUnicodeFold_On_BothQueriesMatch(t *testing.T) {
	items := []accentItem{{id: "moet", name: "Moët & Chandon"}}
	e, err := New(items, WithUnicodeFold())
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"moet", "Moët", "MOET", "Moet"} {
		t.Run(q, func(t *testing.T) {
			r := e.Search(q)
			if len(r) != 1 || r[0].ID != "moet" {
				t.Errorf("query %q: got %v, want [{ID:moet ...}]", q, r)
			}
		})
	}
}

func TestWithUnicodeFold_PrefixCache_HitsAcrossAccents(t *testing.T) {
	items := []accentItem{{id: "moet", name: "Moët & Chandon"}}
	e, err := New(items,
		WithUnicodeFold(),
		WithPrefixCache([]string{"Moët"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	// The cache key must match what Search looks up: query is folded then
	// normalized, so the stored key must be "moet" (folded+normalized),
	// not "moët" (normalized-only). Without folding at build time, every
	// Search call misses the cache even though it still finds the doc via
	// the fallback search path — defeating the purpose of the prewarm.
	if _, ok := e.prefixCache["moet"]; !ok {
		t.Errorf("prefix cache missing expected key %q; keys=%v",
			"moet", keysOf(e.prefixCache))
	}
	for _, q := range []string{"moet", "Moët", "Moet", "MOET"} {
		t.Run(q, func(t *testing.T) {
			r := e.Search(q)
			if len(r) != 1 || r[0].ID != "moet" {
				t.Errorf("query %q: got %v, want 1 hit {ID:moet}", q, r)
			}
		})
	}
}

func keysOf(m map[string][]Result) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

type fbItem struct {
	id, name, group string
}

func (f fbItem) SearchID() string { return f.id }
func (f fbItem) SearchFields() []Field {
	return []Field{
		{Name: "name", Values: []string{f.name}, Weight: 1.0},
		{Name: "comparison_group", Values: []string{f.group}, Weight: 0.8},
	}
}

func newFallbackEngine(t *testing.T) *Engine {
	t.Helper()
	items := []fbItem{
		{id: "heineken", name: "Heineken", group: "lager"},
		{id: "corona", name: "Corona Extra", group: "lager"},
		{id: "guinness", name: "Guinness", group: "stout"},
	}
	e, err := New(items, WithFallbackField("comparison_group"), WithLimit(10))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestSearchWithFallback_PrimaryHits_LevelMinusOne(t *testing.T) {
	e := newFallbackEngine(t)
	results, level := e.SearchWithFallback("heineken", []string{"lager", "beer"})
	if level != -1 {
		t.Errorf("expected level -1 (primary match), got %d", level)
	}
	if len(results) == 0 || results[0].ID != "heineken" {
		t.Errorf("expected heineken first, got %v", results)
	}
}

func TestSearchWithFallback_FirstCascadeHits_LevelZero(t *testing.T) {
	e := newFallbackEngine(t)
	results, level := e.SearchWithFallback("nonexistent-zzz", []string{"lager", "beer"})
	if level != 0 {
		t.Errorf("expected level 0 (cascade[0] match), got %d", level)
	}
	if len(results) < 2 {
		t.Errorf("expected at least 2 lager results, got %d", len(results))
	}
}

func TestSearchWithFallback_DeepCascade_CorrectLevel(t *testing.T) {
	e := newFallbackEngine(t)
	// "nonexistent-zzz" (primary) and "xyz-nomatch" (cascade[0]) match nothing;
	// "stout" (cascade[1]) hits Guinness.
	results, level := e.SearchWithFallback("nonexistent-zzz",
		[]string{"xyz-nomatch", "stout"})
	if level != 1 {
		t.Errorf("expected level 1, got %d", level)
	}
	if len(results) != 1 || results[0].ID != "guinness" {
		t.Errorf("expected guinness, got %v", results)
	}
}

func TestSearchWithFallback_NothingMatches_LevelEqualsLen(t *testing.T) {
	e := newFallbackEngine(t)
	results, level := e.SearchWithFallback("nonexistent-zzz",
		[]string{"nope-1", "nope-2"})
	if level != 2 {
		t.Errorf("expected level 2 (==len(cascade)), got %d", level)
	}
	if len(results) != 0 {
		t.Errorf("expected zero results, got %d", len(results))
	}
}

func TestSearchWithFallback_NilCascade_BehavesLikeSearch(t *testing.T) {
	e := newFallbackEngine(t)
	results, level := e.SearchWithFallback("heineken", nil)
	if level != -1 {
		t.Errorf("expected level -1, got %d", level)
	}
	if len(results) == 0 {
		t.Errorf("expected non-empty results")
	}
}

type filterItem struct {
	id, name string
}

func (f filterItem) SearchID() string { return f.id }
func (f filterItem) SearchFields() []Field {
	return []Field{{Name: "name", Values: []string{f.name}, Weight: 1.0}}
}

func newFilterEngine(t *testing.T, n int) *Engine {
	t.Helper()
	items := make([]filterItem, n)
	for i := range n {
		items[i] = filterItem{id: fmt.Sprintf("d%d", i), name: "lager"}
	}
	e, err := New(items, WithLimit(25))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestWithFilter_NilFilter_SameAsNoOption(t *testing.T) {
	e := newFilterEngine(t, 30)
	a := e.Search("lager")
	b := e.Search("lager", WithFilter(nil))
	if len(a) != len(b) {
		t.Errorf("nil filter changed result count: %d vs %d", len(a), len(b))
	}
}

func TestWithFilter_RejectsHalf(t *testing.T) {
	e := newFilterEngine(t, 30)
	keepEven := func(id string) bool {
		// keep d0, d2, d4...
		return strings.HasSuffix(id, "0") || strings.HasSuffix(id, "2") ||
			strings.HasSuffix(id, "4") || strings.HasSuffix(id, "6") ||
			strings.HasSuffix(id, "8")
	}
	results := e.Search("lager", WithFilter(keepEven))
	for _, r := range results {
		if !keepEven(r.ID) {
			t.Errorf("filter let through odd ID %q", r.ID)
		}
	}
}

func TestWithFilter_AppliedBeforeLimit(t *testing.T) {
	// 30 docs, limit 25, filter rejects 90% — should still return up to 15
	// (the survivors), NOT 1 (which would be 25 unfiltered then filtered).
	e := newFilterEngine(t, 30)
	keepFirstThree := func(id string) bool {
		return id == "d0" || id == "d1" || id == "d2"
	}
	results := e.Search("lager", WithFilter(keepFirstThree))
	if len(results) != 3 {
		t.Errorf("expected 3 survivors, got %d", len(results))
	}
}
