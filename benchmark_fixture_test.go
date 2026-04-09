package xsearch

import (
	"fmt"
	"strings"
	"sync"
)

type benchmarkItem struct {
	id     string
	fields []Field
}

func (i benchmarkItem) SearchID() string      { return i.id }
func (i benchmarkItem) SearchFields() []Field { return i.fields }

type benchmarkScorer []float64

func (s benchmarkScorer) Score(docIndex int) float64 {
	if docIndex < 0 || docIndex >= len(s) {
		return 0
	}
	return s[docIndex]
}

type benchmarkSize struct {
	name string
	docs int
}

type benchmarkEngineVariant struct {
	name            string
	opts            []Option
	withPrefixCache bool
}

type benchmarkCorpus struct {
	items               []benchmarkItem
	exactQuery          string
	prefixQuery         string
	cachedPrefixQuery   string
	typoQuery           string
	missQuery           string
	fallbackExactQuery  string
	fallbackTypoQuery   string
	commonQuery         string
	multiWordQuery      string
	scorer              benchmarkScorer
	negativeBloomProbe  string
}

type benchmarkFamily struct {
	category    string
	descriptors []string
	nouns       []string
	notes       []string
}

var benchmarkSizes = []benchmarkSize{
	{name: "docs_128", docs: 128},
	{name: "docs_2048", docs: 2048},
	{name: "docs_8192", docs: 8192},
}

var benchmarkFamilies = []benchmarkFamily{
	{
		category:    "spirits",
		descriptors: []string{"smoky", "aged", "reserve", "barrel", "single", "cask"},
		nouns:       []string{"whisky", "bourbon", "gin", "rum", "vodka", "tequila"},
		notes:       []string{"oak", "vanilla", "spice", "citrus", "herbal", "pepper"},
	},
	{
		category:    "seltzers",
		descriptors: []string{"bright", "crisp", "sparkling", "cool", "fresh", "zesty"},
		nouns:       []string{"lime", "berry", "grapefruit", "peach", "melon", "coconut"},
		notes:       []string{"bubbly", "clean", "light", "dry", "cane", "tropical"},
	},
	{
		category:    "coffees",
		descriptors: []string{"dark", "silky", "bold", "toasted", "velvet", "morning"},
		nouns:       []string{"espresso", "blend", "roast", "latte", "mocha", "americano"},
		notes:       []string{"cocoa", "hazelnut", "caramel", "roasted", "crema", "molasses"},
	},
	{
		category:    "teas",
		descriptors: []string{"green", "jasmine", "amber", "mountain", "gentle", "wild"},
		nouns:       []string{"sencha", "oolong", "earl", "chai", "matcha", "breakfast"},
		notes:       []string{"floral", "leafy", "citrus", "spiced", "earthy", "honey"},
	},
	{
		category:    "snacks",
		descriptors: []string{"savory", "roasted", "salted", "crispy", "spiced", "golden"},
		nouns:       []string{"almonds", "chips", "pretzels", "crackers", "popcorn", "cashews"},
		notes:       []string{"sea salt", "smoke", "pepper", "garlic", "sesame", "butter"},
	},
	{
		category:    "desserts",
		descriptors: []string{"creamy", "velvet", "midnight", "honey", "frosted", "butter"},
		nouns:       []string{"truffle", "biscuit", "wafer", "cookie", "brownie", "tart"},
		notes:       []string{"cocoa", "toffee", "berry", "vanilla", "cinnamon", "caramel"},
	},
	{
		category:    "mixers",
		descriptors: []string{"tonic", "ginger", "citrus", "dry", "aromatic", "classic"},
		nouns:       []string{"soda", "mixer", "spritz", "cordial", "bitters", "syrup"},
		notes:       []string{"quinine", "lime", "orange", "botanical", "pepper", "clove"},
	},
	{
		category:    "beverages",
		descriptors: []string{"amber", "golden", "session", "hazy", "crisp", "classic"},
		nouns:       []string{"lager", "pilsner", "stout", "porter", "cider", "ale"},
		notes:       []string{"hoppy", "malty", "dry", "fruit", "pine", "smooth"},
	},
}

var benchmarkBrands = []string{
	"Atlas",
	"Northwind",
	"Harbor",
	"Evergreen",
	"Solstice",
	"Cinder",
	"Mariner",
	"Ironwood",
	"Bluebird",
	"Moonrise",
	"Stonepath",
	"Crownline",
}

var benchmarkCollections = []string{
	"Reserve",
	"Select",
	"Craft",
	"Heritage",
	"Studio",
	"Summit",
	"Cellar",
	"Market",
}

var benchmarkRegions = []string{
	"Highland",
	"Coastal",
	"Valley",
	"Harbor",
	"Orchard",
	"Riverside",
	"Summit",
	"Prairie",
	"Forest",
	"Canyon",
	"Lowland",
	"Northshore",
}

var benchmarkSeries = []string{
	"small-batch",
	"single-origin",
	"limited-run",
	"estate",
	"hand-finished",
	"seasonal",
	"chef-curated",
	"barrel-rested",
}

var benchmarkCorpusCache = struct {
	mu   sync.Mutex
	data map[int]*benchmarkCorpus
}{
	data: make(map[int]*benchmarkCorpus),
}

var benchmarkEngineCache = struct {
	mu   sync.Mutex
	data map[string]*Engine
}{
	data: make(map[string]*Engine),
}

func benchmarkCorpusFor(docs int) *benchmarkCorpus {
	benchmarkCorpusCache.mu.Lock()
	defer benchmarkCorpusCache.mu.Unlock()

	if corpus, ok := benchmarkCorpusCache.data[docs]; ok {
		return corpus
	}
	corpus := newBenchmarkCorpus(docs)
	benchmarkCorpusCache.data[docs] = corpus
	return corpus
}

func newBenchmarkCorpus(docs int) *benchmarkCorpus {
	items := make([]benchmarkItem, docs)
	scorer := make(benchmarkScorer, docs)

	brandsLen := len(benchmarkBrands)
	familiesLen := len(benchmarkFamilies)
	collectionsLen := len(benchmarkCollections)
	regionsLen := len(benchmarkRegions)
	seriesLen := len(benchmarkSeries)

	for i := 0; i < docs; i++ {
		family := benchmarkFamilies[i%familiesLen]
		brand := benchmarkBrands[(i/familiesLen)%brandsLen]
		descriptor := family.descriptors[(i/(familiesLen*brandsLen))%len(family.descriptors)]
		noun := family.nouns[(i/(familiesLen*brandsLen*len(family.descriptors)))%len(family.nouns)]
		collection := benchmarkCollections[(i/7)%collectionsLen]
		region := benchmarkRegions[(i/11)%regionsLen]
		series := benchmarkSeries[(i/13)%seriesLen]
		noteA := family.notes[(i/17)%len(family.notes)]
		noteB := family.notes[(i/19+1)%len(family.notes)]

		name := fmt.Sprintf("%s %s %s %s %s", brand, descriptor, noun, collection, region)
		description := fmt.Sprintf(
			"%s %s from the %s %s line with %s and %s notes in a %s finish",
			descriptor,
			noun,
			brand,
			series,
			noteA,
			noteB,
			region,
		)
		tags := []string{
			family.category,
			brand,
			collection,
			series,
			noteA,
			noteB,
		}

		items[i] = benchmarkItem{
			id: fmt.Sprintf("item-%05d", i),
			fields: []Field{
				{Name: "name", Values: []string{name}, Weight: 1.0},
				{Name: "category", Values: []string{family.category}, Weight: 0.6},
				{Name: "region", Values: []string{region}, Weight: 0.3},
				{Name: "description", Values: []string{description}, Weight: 0.5},
				{Name: "tags", Values: tags, Weight: 0.4},
			},
		}
		scorer[i] = float64((i*37)%1000) / 1000.0
	}

	exactName := items[docs/2].fields[0].Values[0]
	commonBrand := strings.ToLower(strings.Fields(items[0].fields[0].Values[0])[0])
	exactWords := strings.Fields(strings.ToLower(exactName))
	multiWordQuery := commonBrand
	if len(exactWords) >= 2 {
		multiWordQuery = strings.Join(exactWords[:2], " ")
	}

	fallbackCategory := benchmarkFamilies[0].category

	return &benchmarkCorpus{
		items:              items,
		exactQuery:         exactName,
		prefixQuery:        benchmarkPrefixQuery(items[0].fields[0].Values[0]),
		cachedPrefixQuery:  strings.ToLower(items[0].fields[0].Values[0][:1]),
		typoQuery:          benchmarkTypoQuery(exactName),
		missQuery:          "zzqxv qjmkp nvthr",
		fallbackExactQuery: fallbackCategory,
		fallbackTypoQuery:  benchmarkTypoQuery(fallbackCategory),
		commonQuery:        commonBrand,
		multiWordQuery:     multiWordQuery,
		scorer:             scorer,
		negativeBloomProbe: "zzqxv",
	}
}

func benchmarkPrefixQuery(s string) string {
	s = normalizeQuery(s)
	if len(s) < 2 {
		return s
	}
	return s[:2]
}

func benchmarkTypoQuery(s string) string {
	words := strings.Fields(normalizeQuery(s))
	for i, word := range words {
		words[i] = benchmarkMutateWord(word)
	}
	return strings.Join(words, " ")
}

func benchmarkMutateWord(word string) string {
	switch {
	case len(word) > 6:
		mid := len(word) / 2
		return word[:mid] + word[mid+1:]
	case len(word) > 4:
		return word[:1] + word[2:] + word[1:2]
	case len(word) > 2:
		return word[:len(word)-1]
	default:
		return word
	}
}

func benchmarkEngineFor(docs int, variant benchmarkEngineVariant) (*Engine, error) {
	key := fmt.Sprintf("%d/%s/pc=%v", docs, variant.name, variant.withPrefixCache)

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
