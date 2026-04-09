---
description: Run the benchmark suite and report performance changes
---

This repo's benchmarks live in the main package (`bench_test.go`), not in a
`./benchmarks` directory. The supported workflow is:

1. Install tools when needed:
   ```bash
   make install-tools
   ```
   This installs `benchstat` and the standalone `pprof` binary.

2. Run a quick local benchmark pass:
   ```bash
   make bench
   ```
   Defaults:
   - package: `.`
   - filter: `.`
   - benchtime: `1s`
   - cpu: `1`

3. Record a benchmark file with commit metadata:
   ```bash
   make bench-record
   ```
   Output is written to `profiles/benchmarks/bench-latest.txt`.

4. Save a baseline before making changes:
   ```bash
   make bench-record
   make bench-save
   ```
   This creates `profiles/benchmarks/bench-prev-<SHA>.txt` and updates
   `profiles/benchmarks/bench-prev.txt`.

5. Compare after code changes:
   ```bash
   make bench-record
   make bench-compare
   ```
   `make bench-compare` prints the SHAs being compared and then runs
   `benchstat`.

6. Compare against a long-lived historical baseline:
   ```bash
   make bench-record
   make bench-compare-baseline
   ```
   This expects `profiles/benchmarks/baseline.txt` to exist.

7. Focus on a subset when you want tighter numbers:
   ```bash
   make bench BENCH_FILTER='^Benchmark(BM25Search|NgramSearch)$'
   make bench-record BENCH_FILTER='^BenchmarkBM25Search$' BENCH_COUNT=10 BENCH_TIME=2s
   ```

8. Profile with `pprof` when a benchmark regresses:
   ```bash
   make bench-pprof-cpu PPROF_NAME=bm25 BENCH_FILTER='^BenchmarkBM25Search$'
   make bench-pprof-top-cpu PPROF_NAME=bm25

   make bench-pprof-mem PPROF_NAME=ngram BENCH_FILTER='^BenchmarkNgramSearch$'
   make bench-pprof-top-mem PPROF_NAME=ngram
   ```
   Profiles are written under `profiles/pprof/`. For an interactive view:
   ```bash
   pprof -http=:0 profiles/pprof/bm25_cpu.prof
   ```
   Memory profiling adds overhead. Use `bench-pprof-mem` for allocation
   analysis, not for comparing `ns/op` against an ordinary `make bench` run.

9. Summarize results carefully:
   - Treat `benchstat` p-values below `0.05` as statistically significant.
   - Call out regressions first, then improvements, then unchanged benchmarks (`~`).
   - Mention the benchmark filter, count, benchtime, and CPU setting used for the run.

10. Same-commit guard:
    If `make bench-compare` says both files are from the same commit, the
    comparison is meaningless. Either:
    - make changes first, then run `make bench-record && make bench-compare`
    - or record a baseline from a different commit with `make bench-record && make bench-save`

## Current Gaps

The workflow is now runnable, but the benchmark suite itself is still narrow.
To improve measurement quality further, prefer adding:

- index-build benchmarks (`New`, snapshot load, snapshot save)
- query-path sub-benchmarks for exact match, typo match, bloom reject, and fallback
- dataset-size variants so small and large corpora are measured separately
- realistic fixtures larger than `testItems()` for search-path benchmarks
