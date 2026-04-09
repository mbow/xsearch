.PHONY: bench bench-record bench-save bench-compare bench-compare-baseline bench-pprof-cpu bench-pprof-mem bench-pprof-top-cpu bench-pprof-top-mem test build run prep install-tools

BENCH_DIR        = profiles/benchmarks
PROFILE_DIR      = profiles/pprof
BENCH_LATEST     = $(BENCH_DIR)/bench-latest.txt
BENCH_PREV       = $(BENCH_DIR)/bench-prev.txt
BENCH_BASELINE   = $(BENCH_DIR)/baseline.txt
BENCH_PKG       ?= .
BENCH_FILTER    ?= .
BENCH_COUNT     ?= 5
BENCH_TIME      ?= 1s
BENCH_CPU       ?= 1
MEMPROFILE_RATE ?= 1
PPROF           ?= pprof
PPROF_NAME      ?= bench
CPU_PROFILE     ?= $(PROFILE_DIR)/$(PPROF_NAME)_cpu.prof
MEM_PROFILE     ?= $(PROFILE_DIR)/$(PPROF_NAME)_mem.prof

install-tools:
	go install golang.org/x/perf/cmd/benchstat@latest
	go install github.com/google/pprof@latest

bench:
	go test -run '^$$' -bench '$(BENCH_FILTER)' -benchmem -benchtime=$(BENCH_TIME) -cpu=$(BENCH_CPU) $(BENCH_PKG)

# Record benchmarks with commit metadata header.
bench-record:
	@mkdir -p $(BENCH_DIR)
	@echo "Running benchmarks (filter=$(BENCH_FILTER), count=$(BENCH_COUNT), time=$(BENCH_TIME), cpu=$(BENCH_CPU), pkg=$(BENCH_PKG))..."
	@SHA=$$(git rev-parse --short HEAD) && \
	BRANCH=$$(git branch --show-current) && \
	DIRTY=$$(git diff --quiet && echo "" || echo " (dirty)") && \
	{ \
		echo "# commit: $$SHA  branch: $$BRANCH$$DIRTY  date: $$(date -Iseconds)"; \
		echo "# filter: $(BENCH_FILTER)  count: $(BENCH_COUNT)  time: $(BENCH_TIME)  cpu: $(BENCH_CPU)  pkg: $(BENCH_PKG)"; \
		echo ""; \
		go test -run '^$$' -bench '$(BENCH_FILTER)' -benchmem -count=$(BENCH_COUNT) -benchtime=$(BENCH_TIME) -cpu=$(BENCH_CPU) $(BENCH_PKG); \
	} > $(BENCH_LATEST)
	@echo "Saved to $(BENCH_LATEST)"

# Save current latest as the comparison baseline (named with commit SHA).
bench-save:
	@if [ ! -f $(BENCH_LATEST) ]; then \
		echo "No $(BENCH_LATEST) found. Run 'make bench-record' first."; \
		exit 1; \
	fi
	@mkdir -p $(BENCH_DIR)
	@sh -eu -c '\
	SHA="$$(awk "NR == 1 { print \$$3 }" "$(BENCH_LATEST)")"; \
	if [ -z "$$SHA" ]; then \
		echo "ERROR: $(BENCH_LATEST) has no commit header. Re-run '\''make bench-record'\''."; \
		exit 1; \
	fi; \
	DEST="$(BENCH_DIR)/bench-prev-$$SHA.txt"; \
	cp "$(BENCH_LATEST)" "$$DEST"; \
	ln -sfn "bench-prev-$$SHA.txt" "$(BENCH_PREV)"; \
	echo "Saved as $$DEST (symlinked from bench-prev.txt)"'

# Compare latest against prev, refusing to compare identical commits.
bench-compare:
	@command -v benchstat >/dev/null 2>&1 || { echo "benchstat not found. Run 'make install-tools'."; exit 1; }
	@if [ ! -L $(BENCH_PREV) ] && [ ! -f $(BENCH_PREV) ]; then \
		echo "No $(BENCH_PREV) found. Run 'make bench-save' then 'make bench-record'."; \
		exit 1; \
	fi
	@if [ ! -f $(BENCH_LATEST) ]; then \
		echo "No $(BENCH_LATEST) found. Run 'make bench-record' first."; \
		exit 1; \
	fi
	@sh -eu -c '\
	PREV_SHA="$$(awk "NR == 1 { print \$$3 }" "$(BENCH_PREV)")"; \
	LATEST_SHA="$$(awk "NR == 1 { print \$$3 }" "$(BENCH_LATEST)")"; \
	if [ -z "$$PREV_SHA" ] || [ -z "$$LATEST_SHA" ]; then \
		echo "ERROR: missing commit header in benchmark files."; \
		exit 1; \
	fi; \
	if [ "$$PREV_SHA" = "$$LATEST_SHA" ]; then \
		echo "ERROR: bench-prev ($$PREV_SHA) and bench-latest ($$LATEST_SHA) are from the same commit."; \
		echo "Nothing to compare - make changes first, then '\''make bench-record'\''."; \
		exit 1; \
	fi; \
	echo "Comparing $$PREV_SHA -> $$LATEST_SHA"; \
	benchstat "$(BENCH_PREV)" "$(BENCH_LATEST)"'

# Compare latest against the original pristine baseline (historical).
bench-compare-baseline:
	@command -v benchstat >/dev/null 2>&1 || { echo "benchstat not found. Run 'make install-tools'."; exit 1; }
	@if [ ! -f $(BENCH_BASELINE) ]; then \
		echo "No $(BENCH_BASELINE) found."; \
		exit 1; \
	fi
	@if [ ! -f $(BENCH_LATEST) ]; then \
		echo "No $(BENCH_LATEST) found. Run 'make bench-record' first."; \
		exit 1; \
	fi
	benchstat $(BENCH_BASELINE) $(BENCH_LATEST)

bench-pprof-cpu:
	@mkdir -p $(PROFILE_DIR)
	@echo "Writing CPU profile to $(CPU_PROFILE) (filter=$(BENCH_FILTER), time=$(BENCH_TIME), cpu=$(BENCH_CPU), pkg=$(BENCH_PKG))..."
	go test -run '^$$' -bench '$(BENCH_FILTER)' -benchmem -count=1 -benchtime=$(BENCH_TIME) -cpu=$(BENCH_CPU) -cpuprofile $(CPU_PROFILE) $(BENCH_PKG)

bench-pprof-mem:
	@mkdir -p $(PROFILE_DIR)
	@echo "Writing memory profile to $(MEM_PROFILE) (filter=$(BENCH_FILTER), time=$(BENCH_TIME), cpu=$(BENCH_CPU), pkg=$(BENCH_PKG))..."
	go test -run '^$$' -bench '$(BENCH_FILTER)' -benchmem -count=1 -benchtime=$(BENCH_TIME) -cpu=$(BENCH_CPU) -memprofile $(MEM_PROFILE) -memprofilerate=$(MEMPROFILE_RATE) $(BENCH_PKG)

bench-pprof-top-cpu:
	@command -v $(PPROF) >/dev/null 2>&1 || { echo "$(PPROF) not found. Run 'make install-tools'."; exit 1; }
	@if [ ! -f $(CPU_PROFILE) ]; then \
		echo "No $(CPU_PROFILE) found. Run 'make bench-pprof-cpu PPROF_NAME=<name> BENCH_FILTER=<regex>' first."; \
		exit 1; \
	fi
	$(PPROF) -top $(CPU_PROFILE)

bench-pprof-top-mem:
	@command -v $(PPROF) >/dev/null 2>&1 || { echo "$(PPROF) not found. Run 'make install-tools'."; exit 1; }
	@if [ ! -f $(MEM_PROFILE) ]; then \
		echo "No $(MEM_PROFILE) found. Run 'make bench-pprof-mem PPROF_NAME=<name> BENCH_FILTER=<regex>' first."; \
		exit 1; \
	fi
	$(PPROF) -top -alloc_space $(MEM_PROFILE)

test:
	go test ./...

build:
	go build -o search bin/main.go

run:
	go run main.go

prep: test bench build
