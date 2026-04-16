package xsearch

import "testing"

func TestSnapshotRoundTrip(t *testing.T) {
	engine, err := New(testItems(), WithBloom(100), WithFallbackField("category"), WithLimit(5))
	if err != nil {
		t.Fatal(err)
	}
	before := engine.Search("bud")
	snapshot, err := engine.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := NewFromSnapshot(snapshot, testItems(), WithLimit(5))
	if err != nil {
		t.Fatal(err)
	}
	after := restored.Search("bud")
	if len(before) != len(after) {
		t.Fatalf("result count mismatch: %d != %d", len(before), len(after))
	}
	for i := range before {
		if before[i].ID != after[i].ID || before[i].MatchType != after[i].MatchType {
			t.Fatalf("result[%d] mismatch: before=%+v after=%+v", i, before[i], after[i])
		}
	}
}

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

	restored, err := NewFromSnapshot(data, items, WithPrefixCache([]string{"b", "ni"}))
	if err != nil {
		t.Fatalf("NewFromSnapshot with WithPrefixCache: %v", err)
	}

	results := restored.Search("b")
	if len(results) == 0 {
		t.Fatal("expected cached prefix results from restored engine")
	}

	results = restored.Search("nike")
	if len(results) == 0 {
		t.Fatal("expected normal search results from restored engine")
	}
}

func TestSnapshotWithScopes(t *testing.T) {
	items := testItems()
	engine, err := New(items)
	if err != nil {
		t.Fatal(err)
	}

	data, err := engine.Snapshot()
	if err != nil {
		t.Fatal(err)
	}

	restored, err := NewFromSnapshot(data, items, WithScopes(map[string][]string{
		"beer":  {"budweiser", "bud-light", "funky-buddha"},
		"shoes": {"nike-air-max", "nike-dunk"},
	}))
	if err != nil {
		t.Fatalf("NewFromSnapshot with WithScopes: %v", err)
	}

	results := restored.Search("nike", WithScope("shoes"))
	if len(results) == 0 {
		t.Fatal("expected scoped search results from restored engine")
	}

	for _, result := range results {
		if result.ID != "nike-air-max" && result.ID != "nike-dunk" {
			t.Fatalf("unexpected scoped result %q", result.ID)
		}
	}
}

func TestSnapshotVersionRejection(t *testing.T) {
	engine, err := New(testItems())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := engine.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	snapshot[4] = 99
	if _, err := NewFromSnapshot(snapshot, testItems()); err == nil {
		t.Fatal("expected version rejection")
	}
}
