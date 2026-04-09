---
description: Run the benchmark suite and report performance changes
---

This repo's benchmarks live in the main package and are organized into four
groups:

- `BenchmarkComponent`: hot helpers and internal indexes
- `BenchmarkBuildEngine`, `BenchmarkSnapshotEncode`, `BenchmarkSnapshotLoad`: build and snapshot costs
- `BenchmarkSearch`: end-to-end search workloads across multiple corpus sizes
- `BenchmarkParallelSearch`: parallel search scaling on the large corpus

The supported workflow is:

1. Install tools when needed:
   ```bash
   make install-tools
   ```
   This installs `benchstat` and the standalone `pprof` binary.

2. Run a quick local benchmark pass:
   ```bash
   make bench
   ```
   This runs the full suite. For targeted runs:
   ```bash
   make bench-component
   make bench-build
   make bench-search
   make bench-scale
   make bench-hotpaths
   ```
   Defaults for all targets:
   - package: `.`
   - filter: `.`
   - benchtime: `1s`
   - cpu: `1`

3. Record a benchmark file with commit metadata:
   ```bash
   make bench-record
   ```
   Output is written to `profiles/benchmarks/bench-latest.txt`.

4. Update the published README performance snapshot when needed:
   ```bash
   make bench-readme
   ```
   This rewrites the generated `## Performance` section in `README.md` using
   medians from the current `profiles/benchmarks/bench-latest.txt`.

   For a one-shot workflow:
   ```bash
   make bench-publish
   ```
   This runs `bench-record` and then updates the README.

5. Save a baseline before making changes:
   ```bash
   make bench-record
   make bench-save
   ```
   This creates `profiles/benchmarks/bench-prev-<SHA>.txt` and updates
   `profiles/benchmarks/bench-prev.txt`.

6. Compare after code changes:
   ```bash
   make bench-record
   make bench-compare
   ```
   `make bench-compare` prints the SHAs being compared and then runs
   `benchstat`.

7. Compare against a long-lived historical baseline:
   ```bash
   make bench-record
   make bench-compare-baseline
   ```
   This expects `profiles/benchmarks/baseline.txt` to exist.

8. Focus on a subset when you want tighter numbers:
   ```bash
   make bench-hotpaths BENCH_TIME=2s
   make bench-record BENCH_FILTER='^BenchmarkSearch$$' BENCH_COUNT=10 BENCH_TIME=200ms
   make bench-record BENCH_FILTER='^BenchmarkSearch$$/typo/default/docs_8192$$' BENCH_COUNT=12 BENCH_TIME=2s
   ```

9. Profile with `pprof` when a benchmark regresses:
   ```bash
   make bench-pprof-cpu PPROF_NAME=search-typo BENCH_FILTER='^BenchmarkSearch$$/typo/default/docs_8192$$' BENCH_TIME=2s
   make bench-pprof-top-cpu PPROF_NAME=search-typo

   make bench-pprof-mem PPROF_NAME=build-full BENCH_FILTER='^BenchmarkBuildEngine$$/full/docs_8192$$' BENCH_TIME=2s
   make bench-pprof-top-mem PPROF_NAME=build-full
   ```
   Profiles are written under `profiles/pprof/`. For an interactive view:
   ```bash
   pprof -http=:0 profiles/pprof/search-typo_cpu.prof
   ```
   Memory profiling adds overhead. Use `bench-pprof-mem` for allocation
   analysis, not for comparing `ns/op` against an ordinary `make bench` run.

## Suite Coverage

The current suite covers:

- helper costs for bloom, trigram extraction, tokenization, BM25, ngram, and fallback
- engine construction with `default`, `bloom`, `fallback`, and `full` variants
- snapshot encode/load with and without bloom/fallback
- end-to-end search workloads for exact, prefix, typo, miss, bloom reject, fallback exact, fallback fuzzy, scorer blending, result-limit pressure, and multiword highlight-heavy queries
- corpus sizes `128`, `2048`, and `8192`
- parallel search scaling on the large corpus

For routine regression tracking, prefer:

- `make bench-record BENCH_FILTER='^BenchmarkComponent$$' BENCH_COUNT=10 BENCH_TIME=200ms`
- `make bench-record BENCH_FILTER='^BenchmarkBuildEngine$$|^BenchmarkSnapshot(Encode|Load)$$' BENCH_COUNT=6 BENCH_TIME=200ms`
- `make bench-record BENCH_FILTER='^BenchmarkSearch$$' BENCH_COUNT=8 BENCH_TIME=200ms`
- `make bench-scale BENCH_TIME=500ms`

10. Summarize results carefully:
   - Treat `benchstat` p-values below `0.05` as statistically significant.
   - Call out regressions first, then improvements, then unchanged benchmarks (`~`).
   - Mention the benchmark filter, count, benchtime, and CPU setting used for the run.
   - If the run is intended to update public docs, run `make bench-readme` and
     mention that `README.md` was refreshed from the latest benchmark snapshot.

11. Same-commit guard:
    If `make bench-compare` says both files are from the same commit, the
    comparison is meaningless. Either:
    - make changes first, then run `make bench-record && make bench-compare`
    - or record a baseline from a different commit with `make bench-record && make bench-save`

## Remaining Gaps

This is broad enough for serious regression tracking, but there are still two
future expansions worth considering:

- trace-driven workloads from production-like query logs if you have them
- long-running soak benchmarks for cache behavior and GC pacing under repeated mixed-query traffic
