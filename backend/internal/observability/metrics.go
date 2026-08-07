package observability

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// A small Prometheus exposition implementation, written against the text format
// (version 0.0.4) rather than pulled in as a dependency.
//
// The obvious alternative is github.com/prometheus/client_golang, and for a
// service that expects to grow it would be the right call. It is not used here
// for one concrete reason: it drags in five transitive modules, two of which
// (google.golang.org/protobuf, golang.org/x/sys) are hosted outside GitHub, and
// this project is handed over to a team that has to be able to rebuild it from
// a clean checkout in one command, forever, without surprises in go.sum. The
// backend already speaks Redis over a hand-written RESP client for the same
// reason, so this stays consistent with the codebase it lives in.
//
// The cost is real and worth stating: no free Go runtime metrics, and no
// exemplars or native histograms. Container-level CPU and memory come from the
// container runtime anyway, and the metrics that matter here are domain ones.
//
// What is exported is a strict subset of the Prometheus text format, and the
// output is verified against promtool in the acceptance criteria.

// Registry owns every metric of a process.
type Registry struct {
	mu         sync.Mutex
	collectors []collector
}

type collector interface {
	name() string
	write(w io.Writer)
}

// DefaultRegistry is the registry served by GET /metrics.
var DefaultRegistry = &Registry{}

func (r *Registry) register(c collector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.collectors {
		if existing.name() == c.name() {
			// Registering the same metric twice is a programming error that
			// would silently produce a duplicated exposition, which Prometheus
			// rejects. Fail loudly at startup instead.
			panic("observability: metric already registered: " + c.name())
		}
	}
	r.collectors = append(r.collectors, c)
}

// Render renders the whole registry in the Prometheus text exposition format.
// Output is sorted so that a diff between two scrapes is readable by a human.
func (r *Registry) Render(w io.Writer) {
	r.mu.Lock()
	collectors := make([]collector, len(r.collectors))
	copy(collectors, r.collectors)
	r.mu.Unlock()

	sort.Slice(collectors, func(i, j int) bool { return collectors[i].name() < collectors[j].name() })
	for _, c := range collectors {
		c.write(w)
	}
}

// ContentType is the media type of the exposition format produced by Render.
const ContentType = "text/plain; version=0.0.4; charset=utf-8"

type metadata struct {
	metricName string
	help       string
	labelNames []string
}

func (m metadata) name() string { return m.metricName }

func (m metadata) writeHeader(w io.Writer, metricType string) {
	fmt.Fprintf(w, "# HELP %s %s\n", m.metricName, escapeHelp(m.help))
	fmt.Fprintf(w, "# TYPE %s %s\n", m.metricName, metricType)
}

// seriesKey joins label values with a separator that cannot appear in a label
// value once escaped, so two different label sets can never collide.
const seriesSeparator = "\x00"

func (m metadata) formatLabels(values []string, extraName, extraValue string) string {
	if len(values) == 0 && extraName == "" {
		return ""
	}
	pairs := make([]string, 0, len(values)+1)
	for index, value := range values {
		if index >= len(m.labelNames) {
			break
		}
		// %q would escape a second time on top of escapeLabelValue, so the
		// quotes are written by hand.
		pairs = append(pairs, m.labelNames[index]+`="`+escapeLabelValue(value)+`"`)
	}
	if extraName != "" {
		pairs = append(pairs, extraName+`="`+escapeLabelValue(extraValue)+`"`)
	}
	return "{" + strings.Join(pairs, ",") + "}"
}

// CounterVec is a set of monotonically increasing counters.
type CounterVec struct {
	metadata
	mu     sync.Mutex
	values map[string]float64
}

func NewCounterVec(name, help string, labelNames ...string) *CounterVec {
	counter := &CounterVec{
		metadata: metadata{metricName: name, help: help, labelNames: labelNames},
		values:   map[string]float64{},
	}
	DefaultRegistry.register(counter)
	return counter
}

func (c *CounterVec) Inc(labelValues ...string) { c.Add(1, labelValues...) }

func (c *CounterVec) Add(delta float64, labelValues ...string) {
	if delta < 0 || math.IsNaN(delta) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[strings.Join(labelValues, seriesSeparator)] += delta
}

func (c *CounterVec) write(w io.Writer) {
	c.mu.Lock()
	snapshot := make(map[string]float64, len(c.values))
	for key, value := range c.values {
		snapshot[key] = value
	}
	c.mu.Unlock()

	c.writeHeader(w, "counter")
	for _, key := range sortedKeys(snapshot) {
		labels := c.formatLabels(splitSeries(key), "", "")
		fmt.Fprintf(w, "%s%s %s\n", c.metricName, labels, formatValue(snapshot[key]))
	}
}

// GaugeVec is a set of values that can go up and down. Gauges here are written
// by background collectors, never by request handlers.
type GaugeVec struct {
	metadata
	mu     sync.Mutex
	values map[string]float64
}

func NewGaugeVec(name, help string, labelNames ...string) *GaugeVec {
	gauge := &GaugeVec{
		metadata: metadata{metricName: name, help: help, labelNames: labelNames},
		values:   map[string]float64{},
	}
	DefaultRegistry.register(gauge)
	return gauge
}

func (g *GaugeVec) Set(value float64, labelValues ...string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.values[strings.Join(labelValues, seriesSeparator)] = value
}

func (g *GaugeVec) Add(delta float64, labelValues ...string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.values[strings.Join(labelValues, seriesSeparator)] += delta
}

// Reset drops every series. Background collectors call it before republishing a
// snapshot so that a label combination that disappeared (a task status with no
// rows left) does not linger at its last value forever.
func (g *GaugeVec) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.values = map[string]float64{}
}

func (g *GaugeVec) write(w io.Writer) {
	g.mu.Lock()
	snapshot := make(map[string]float64, len(g.values))
	for key, value := range g.values {
		snapshot[key] = value
	}
	g.mu.Unlock()

	g.writeHeader(w, "gauge")
	for _, key := range sortedKeys(snapshot) {
		labels := g.formatLabels(splitSeries(key), "", "")
		fmt.Fprintf(w, "%s%s %s\n", g.metricName, labels, formatValue(snapshot[key]))
	}
}

// HistogramVec records a distribution in fixed buckets.
type HistogramVec struct {
	metadata
	buckets []float64
	mu      sync.Mutex
	series  map[string]*histogramSeries
}

type histogramSeries struct {
	counts []uint64
	sum    float64
	count  uint64
}

func NewHistogramVec(name, help string, buckets []float64, labelNames ...string) *HistogramVec {
	ordered := make([]float64, len(buckets))
	copy(ordered, buckets)
	sort.Float64s(ordered)
	histogram := &HistogramVec{
		metadata: metadata{metricName: name, help: help, labelNames: labelNames},
		buckets:  ordered,
		series:   map[string]*histogramSeries{},
	}
	DefaultRegistry.register(histogram)
	return histogram
}

func (h *HistogramVec) Observe(value float64, labelValues ...string) {
	if math.IsNaN(value) {
		return
	}
	key := strings.Join(labelValues, seriesSeparator)
	h.mu.Lock()
	defer h.mu.Unlock()
	series, ok := h.series[key]
	if !ok {
		series = &histogramSeries{counts: make([]uint64, len(h.buckets))}
		h.series[key] = series
	}
	series.sum += value
	series.count++
	for index, upperBound := range h.buckets {
		if value <= upperBound {
			series.counts[index]++
		}
	}
}

func (h *HistogramVec) write(w io.Writer) {
	h.mu.Lock()
	keys := make([]string, 0, len(h.series))
	snapshot := make(map[string]histogramSeries, len(h.series))
	for key, series := range h.series {
		counts := make([]uint64, len(series.counts))
		copy(counts, series.counts)
		snapshot[key] = histogramSeries{counts: counts, sum: series.sum, count: series.count}
		keys = append(keys, key)
	}
	h.mu.Unlock()
	sort.Strings(keys)

	h.writeHeader(w, "histogram")
	for _, key := range keys {
		series := snapshot[key]
		values := splitSeries(key)
		// Prometheus histogram buckets are cumulative, and Observe already
		// increments every bucket a value falls into, so counts are cumulative.
		for index, upperBound := range h.buckets {
			labels := h.formatLabels(values, "le", formatValue(upperBound))
			fmt.Fprintf(w, "%s_bucket%s %d\n", h.metricName, labels, series.counts[index])
		}
		fmt.Fprintf(w, "%s_bucket%s %d\n", h.metricName, h.formatLabels(values, "le", "+Inf"), series.count)
		fmt.Fprintf(w, "%s_sum%s %s\n", h.metricName, h.formatLabels(values, "", ""), formatValue(series.sum))
		fmt.Fprintf(w, "%s_count%s %d\n", h.metricName, h.formatLabels(values, "", ""), series.count)
	}
}

func sortedKeys(values map[string]float64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func splitSeries(key string) []string {
	if key == "" {
		return nil
	}
	return strings.Split(key, seriesSeparator)
}

func formatValue(value float64) string {
	if math.IsInf(value, 1) {
		return "+Inf"
	}
	if math.IsInf(value, -1) {
		return "-Inf"
	}
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func escapeHelp(help string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`).Replace(help)
}

func escapeLabelValue(value string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`).Replace(value)
}
