package observability

import (
	"bytes"
	"strings"
	"testing"
)

func render(t *testing.T, registry *Registry) string {
	t.Helper()
	var buffer bytes.Buffer
	registry.Render(&buffer)
	return buffer.String()
}

// newTestRegistry swaps the package-level registry so a test can register its
// own metrics without colliding with the real definitions.
func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	previous := DefaultRegistry
	registry := &Registry{}
	DefaultRegistry = registry
	t.Cleanup(func() { DefaultRegistry = previous })
	return registry
}

func TestCounterExposition(t *testing.T) {
	registry := newTestRegistry(t)
	counter := NewCounterVec("codepilot_test_total", "A test counter.", "route", "status")

	counter.Inc("/health", "200")
	counter.Inc("/health", "200")
	counter.Add(3, "/ready", "503")
	counter.Add(-1, "/ready", "503") // negative deltas are ignored, not applied

	expected := strings.Join([]string{
		"# HELP codepilot_test_total A test counter.",
		"# TYPE codepilot_test_total counter",
		`codepilot_test_total{route="/health",status="200"} 2`,
		`codepilot_test_total{route="/ready",status="503"} 3`,
		"",
	}, "\n")
	if got := render(t, registry); got != expected {
		t.Fatalf("exposition =\n%s\nwant\n%s", got, expected)
	}
}

func TestGaugeSetAndReset(t *testing.T) {
	registry := newTestRegistry(t)
	gauge := NewGaugeVec("codepilot_test_gauge", "A test gauge.", "component")

	gauge.Set(1, "storage")
	gauge.Set(0, "rag")
	if got := render(t, registry); !strings.Contains(got, `codepilot_test_gauge{component="rag"} 0`) {
		t.Fatalf("exposition = %s", got)
	}

	// A label combination that disappears must not linger at its last value.
	gauge.Reset()
	gauge.Set(1, "storage")
	got := render(t, registry)
	if strings.Contains(got, `component="rag"`) {
		t.Fatalf("reset did not drop the stale series: %s", got)
	}
	if !strings.Contains(got, `codepilot_test_gauge{component="storage"} 1`) {
		t.Fatalf("exposition = %s", got)
	}
}

func TestHistogramBucketsAreCumulative(t *testing.T) {
	registry := newTestRegistry(t)
	histogram := NewHistogramVec(
		"codepilot_test_duration_seconds", "A test histogram.",
		[]float64{1, 5, 10}, "task_type",
	)

	histogram.Observe(0.5, "plan")
	histogram.Observe(3, "plan")
	histogram.Observe(30, "plan")

	expected := strings.Join([]string{
		"# HELP codepilot_test_duration_seconds A test histogram.",
		"# TYPE codepilot_test_duration_seconds histogram",
		`codepilot_test_duration_seconds_bucket{task_type="plan",le="1"} 1`,
		`codepilot_test_duration_seconds_bucket{task_type="plan",le="5"} 2`,
		`codepilot_test_duration_seconds_bucket{task_type="plan",le="10"} 2`,
		`codepilot_test_duration_seconds_bucket{task_type="plan",le="+Inf"} 3`,
		`codepilot_test_duration_seconds_sum{task_type="plan"} 33.5`,
		`codepilot_test_duration_seconds_count{task_type="plan"} 3`,
		"",
	}, "\n")
	if got := render(t, registry); got != expected {
		t.Fatalf("exposition =\n%s\nwant\n%s", got, expected)
	}
}

func TestLabelValuesAreEscapedExactlyOnce(t *testing.T) {
	registry := newTestRegistry(t)
	counter := NewCounterVec("codepilot_test_escaping_total", "Escaping\ntest.", "detail")

	counter.Inc(`quote " backslash \ newline` + "\n")

	got := render(t, registry)
	if !strings.Contains(got, `# HELP codepilot_test_escaping_total Escaping\ntest.`) {
		t.Fatalf("help line is not escaped: %s", got)
	}
	if !strings.Contains(got, `{detail="quote \" backslash \\ newline\n"}`) {
		t.Fatalf("label value is not escaped exactly once: %s", got)
	}
}

func TestRegistryOutputIsSortedAndStable(t *testing.T) {
	registry := newTestRegistry(t)
	second := NewCounterVec("codepilot_test_b_total", "B.")
	first := NewCounterVec("codepilot_test_a_total", "A.")
	second.Inc()
	first.Inc()

	got := render(t, registry)
	if strings.Index(got, "codepilot_test_a_total") > strings.Index(got, "codepilot_test_b_total") {
		t.Fatalf("metrics are not sorted by name: %s", got)
	}
	if got != render(t, registry) {
		t.Fatal("two consecutive renderings differ")
	}
}

func TestDuplicateRegistrationPanics(t *testing.T) {
	newTestRegistry(t)
	NewCounterVec("codepilot_test_duplicate_total", "First.")

	defer func() {
		if recover() == nil {
			t.Fatal("registering the same metric name twice must panic at startup")
		}
	}()
	NewCounterVec("codepilot_test_duplicate_total", "Second.")
}

func TestUpstreamOutcomeVocabulary(t *testing.T) {
	for status, expected := range map[int]string{200: "success", 404: "client_error", 502: "server_error"} {
		if got := UpstreamOutcome(status); got != expected {
			t.Fatalf("UpstreamOutcome(%d) = %q, want %q", status, got, expected)
		}
	}
}
