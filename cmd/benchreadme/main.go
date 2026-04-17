package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	readmeStartMarker = "<!-- BENCHMARKS:START -->"
	readmeEndMarker   = "<!-- BENCHMARKS:END -->"
)

var metricOrder = []string{"ns/op", "MB/s", "B/op", "allocs/op"}

type benchMetadata struct {
	Commit string
	Branch string
	Date   string
	Filter string
	Count  string
	Time   string
	CPU    string
	Pkg    string
}

type benchRow struct {
	Name    string
	Metrics map[string][]float64
}

type benchGroup struct {
	Name  string
	Rows  []*benchRow
	index map[string]*benchRow
}

type benchSummary struct {
	Metadata benchMetadata
	Groups   []*benchGroup
	index    map[string]*benchGroup
}

func main() {
	var benchPath string
	var readmePath string

	flag.StringVar(&benchPath, "bench", "profiles/benchmarks/bench-latest.txt", "path to benchmark results")
	flag.StringVar(&readmePath, "readme", "README.md", "path to README to update")
	flag.Parse()

	summary, err := parseBenchFile(benchPath)
	if err != nil {
		fatal(err)
	}

	readme, err := os.ReadFile(readmePath)
	if err != nil {
		fatal(fmt.Errorf("read %s: %w", readmePath, err))
	}

	section, err := renderSection(summary, benchPath)
	if err != nil {
		fatal(err)
	}

	updated, err := replaceReadmeSection(readme, section)
	if err != nil {
		fatal(err)
	}

	if bytes.Equal(readme, updated) {
		fmt.Printf("%s is already up to date\n", readmePath)
		return
	}

	if err := os.WriteFile(readmePath, updated, 0o644); err != nil {
		fatal(fmt.Errorf("write %s: %w", readmePath, err))
	}

	fmt.Printf("Updated %s from %s\n", readmePath, benchPath)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func parseBenchFile(path string) (*benchSummary, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	summary := &benchSummary{
		index: make(map[string]*benchGroup),
	}

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "# commit: "):
			if err := parseCommitHeader(line, &summary.Metadata); err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
			}
		case strings.HasPrefix(line, "# filter: "):
			if err := parseConfigHeader(line, &summary.Metadata); err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
			}
		case strings.HasPrefix(line, "Benchmark"):
			if err := parseBenchmarkLine(line, summary); err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	if len(summary.Groups) == 0 {
		return nil, fmt.Errorf("no benchmark rows found in %s", path)
	}
	if summary.Metadata.Commit == "" {
		return nil, fmt.Errorf("missing benchmark metadata header in %s", path)
	}
	return summary, nil
}

func parseCommitHeader(line string, md *benchMetadata) error {
	const prefix = "# commit: "
	line = strings.TrimPrefix(line, prefix)

	branchIdx := strings.Index(line, "  branch: ")
	dateIdx := strings.Index(line, "  date: ")
	if branchIdx < 0 || dateIdx < 0 || dateIdx <= branchIdx {
		return fmt.Errorf("invalid commit header %q", line)
	}

	md.Commit = strings.TrimSpace(line[:branchIdx])
	md.Branch = strings.TrimSpace(line[branchIdx+len("  branch: "):dateIdx])
	md.Date = strings.TrimSpace(line[dateIdx+len("  date: "):])
	return nil
}

func parseConfigHeader(line string, md *benchMetadata) error {
	const prefix = "# filter: "
	line = strings.TrimPrefix(line, prefix)

	countIdx := strings.Index(line, "  count: ")
	timeIdx := strings.Index(line, "  time: ")
	cpuIdx := strings.Index(line, "  cpu: ")
	pkgIdx := strings.Index(line, "  pkg: ")
	if countIdx < 0 || timeIdx < 0 || cpuIdx < 0 || pkgIdx < 0 {
		return fmt.Errorf("invalid config header %q", line)
	}

	md.Filter = strings.TrimSpace(line[:countIdx])
	md.Count = strings.TrimSpace(line[countIdx+len("  count: "):timeIdx])
	md.Time = strings.TrimSpace(line[timeIdx+len("  time: "):cpuIdx])
	md.CPU = strings.TrimSpace(line[cpuIdx+len("  cpu: "):pkgIdx])
	md.Pkg = strings.TrimSpace(line[pkgIdx+len("  pkg: "):])
	return nil
}

func parseBenchmarkLine(line string, summary *benchSummary) error {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return fmt.Errorf("invalid benchmark row %q", line)
	}

	name := trimGOMAXPROCSSuffix(fields[0])
	groupName, displayName := splitBenchmarkName(name)
	group := ensureGroup(summary, groupName)
	row := ensureRow(group, displayName)

	for i := 2; i+1 < len(fields); i += 2 {
		value, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return fmt.Errorf("parse metric value %q: %w", fields[i], err)
		}
		unit := fields[i+1]
		row.Metrics[unit] = append(row.Metrics[unit], value)
	}
	return nil
}

func trimGOMAXPROCSSuffix(name string) string {
	if idx := strings.LastIndex(name, "-"); idx >= 0 {
		if _, err := strconv.Atoi(name[idx+1:]); err == nil {
			return name[:idx]
		}
	}
	return name
}

func splitBenchmarkName(name string) (string, string) {
	parts := strings.Split(name, "/")
	if len(parts) == 1 {
		return parts[0], parts[0]
	}
	return parts[0], strings.Join(parts[1:], "/")
}

func ensureGroup(summary *benchSummary, name string) *benchGroup {
	if group, ok := summary.index[name]; ok {
		return group
	}
	group := &benchGroup{
		Name:  name,
		index: make(map[string]*benchRow),
	}
	summary.index[name] = group
	summary.Groups = append(summary.Groups, group)
	return group
}

func ensureRow(group *benchGroup, name string) *benchRow {
	if row, ok := group.index[name]; ok {
		return row
	}
	row := &benchRow{
		Name:    name,
		Metrics: make(map[string][]float64),
	}
	group.index[name] = row
	group.Rows = append(group.Rows, row)
	return row
}

func renderSection(summary *benchSummary, benchPath string) ([]byte, error) {
	var out bytes.Buffer

	fmt.Fprintf(&out, "_Generated from `%s` via `make bench-readme`. Metric cells are medians across the recorded samples._\n\n", benchPath)
	fmt.Fprintf(&out, "- Commit: `%s`\n", summary.Metadata.Commit)
	fmt.Fprintf(&out, "- Branch: `%s`\n", summary.Metadata.Branch)
	fmt.Fprintf(&out, "- Date: `%s`\n", summary.Metadata.Date)
	fmt.Fprintf(&out, "- Filter: `%s`\n", summary.Metadata.Filter)
	fmt.Fprintf(&out, "- Count: `%s`\n", summary.Metadata.Count)
	fmt.Fprintf(&out, "- Time: `%s`\n", summary.Metadata.Time)
	fmt.Fprintf(&out, "- CPU: `%s`\n", summary.Metadata.CPU)
	fmt.Fprintf(&out, "- Package: `%s`\n", summary.Metadata.Pkg)

	for _, group := range summary.Groups {
		metrics := orderedMetrics(group)
		if len(metrics) == 0 {
			continue
		}

		fmt.Fprintf(&out, "\n### %s\n\n", groupTitle(group.Name))
		fmt.Fprint(&out, "| Benchmark |")
		for _, metric := range metrics {
			fmt.Fprintf(&out, " %s |", metric)
		}
		fmt.Fprint(&out, "\n| --- |")
		for range metrics {
			fmt.Fprint(&out, " ---: |")
		}
		fmt.Fprint(&out, "\n")

		for _, row := range group.Rows {
			fmt.Fprintf(&out, "| `%s` |", row.Name)
			for _, metric := range metrics {
				fmt.Fprintf(&out, " %s |", formatMetric(metric, median(row.Metrics[metric])))
			}
			fmt.Fprint(&out, "\n")
		}
	}

	return bytes.TrimRight(out.Bytes(), "\n"), nil
}

func orderedMetrics(group *benchGroup) []string {
	seen := make(map[string]bool)
	for _, row := range group.Rows {
		for metric := range row.Metrics {
			seen[metric] = true
		}
	}

	var metrics []string
	for _, metric := range metricOrder {
		if seen[metric] {
			metrics = append(metrics, metric)
			delete(seen, metric)
		}
	}
	if len(seen) > 0 {
		var extra []string
		for metric := range seen {
			extra = append(extra, metric)
		}
		sort.Strings(extra)
		metrics = append(metrics, extra...)
	}
	return metrics
}

func groupTitle(name string) string {
	switch name {
	case "BenchmarkComponent":
		return "Component"
	case "BenchmarkBuildEngine":
		return "Build Engine"
	case "BenchmarkSnapshotEncode":
		return "Snapshot Encode"
	case "BenchmarkSnapshotLoad":
		return "Snapshot Load"
	case "BenchmarkSearch":
		return "Search"
	case "BenchmarkParallelSearch":
		return "Parallel Search"
	default:
		return strings.TrimPrefix(name, "Benchmark")
	}
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func formatMetric(unit string, value float64) string {
	switch unit {
	case "B/op", "allocs/op":
		return strconv.FormatInt(int64(value+0.5), 10)
	case "ns/op":
		switch {
		case value >= 1000:
			return fmt.Sprintf("%.0f", value)
		case value >= 100:
			return fmt.Sprintf("%.1f", value)
		case value >= 10:
			return fmt.Sprintf("%.2f", value)
		default:
			return fmt.Sprintf("%.3f", value)
		}
	case "MB/s":
		return fmt.Sprintf("%.2f", value)
	default:
		if value == float64(int64(value)) {
			return strconv.FormatInt(int64(value), 10)
		}
		return fmt.Sprintf("%.3f", value)
	}
}

func replaceReadmeSection(readme, section []byte) ([]byte, error) {
	start := bytes.Index(readme, []byte(readmeStartMarker))
	end := bytes.Index(readme, []byte(readmeEndMarker))
	if start < 0 || end < 0 || end < start {
		return nil, errors.New("README.md is missing benchmark section markers")
	}

	start += len(readmeStartMarker)

	var out bytes.Buffer
	out.Write(readme[:start])
	out.WriteByte('\n')
	out.Write(section)
	out.WriteByte('\n')
	out.Write(readme[end:])
	return out.Bytes(), nil
}
