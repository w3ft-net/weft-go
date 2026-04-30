package weft

import (
	"context"
	"runtime"
	gormetrics "runtime/metrics"
	"sort"
	"time"
)

// DefaultRuntimeStatsInterval is the recommended emission cadence
// for RuntimeStats. Matches weftd's OS-metrics rhythm (30s) so
// dashboards align.
const DefaultRuntimeStatsInterval = 30 * time.Second

// RuntimeStats launches a goroutine that emits one Go-runtime
// telemetry record per interval until ctx is cancelled. service is
// a short identifier ("coordinator", "shard", "api") that becomes
// the leaf of the record's app field — emissions go to
// "<service>/runtime" so runtime telemetry lives alongside the
// service's regular events without polluting them. interval ≤ 0
// falls back to DefaultRuntimeStatsInterval.
//
// The method returns immediately; callers don't manage the
// goroutine lifecycle directly — cancel ctx to stop.
func (c *Client) RuntimeStats(ctx context.Context, service string, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultRuntimeStatsInterval
	}
	go c.runRuntimeStats(ctx, service, interval)
}

func (c *Client) runRuntimeStats(ctx context.Context, service string, interval time.Duration) {
	app := service + "/runtime"
	startedAt := time.Now()
	t := time.NewTicker(interval)
	defer t.Stop()

	// Emit one immediately so dashboards have a value before the
	// first interval elapses.
	c.emitRuntimeStats(app, service, startedAt)

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.emitRuntimeStats(app, service, startedAt)
		}
	}
}

func (c *Client) emitRuntimeStats(app, service string, startedAt time.Time) {
	now := time.Now()

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	pauseUs := pausePercentilesMicroseconds(&ms)

	record := map[string]any{
		"type":             "go_runtime",
		"ts":               now.UTC().Format(time.RFC3339Nano),
		"service":          service,
		"uptime_s":         now.Sub(startedAt).Seconds(),
		"goroutines":       runtime.NumGoroutine(),
		"fds_open":         openFDs(),
		"heap_alloc_mb":    bytesToMB(ms.HeapAlloc),
		"heap_sys_mb":      bytesToMB(ms.HeapSys),
		"heap_inuse_mb":    bytesToMB(ms.HeapInuse),
		"stack_inuse_mb":   bytesToMB(ms.StackInuse),
		"gc_pauses_p50_us": pauseUs.p50,
		"gc_pauses_p95_us": pauseUs.p95,
		"gc_pauses_p99_us": pauseUs.p99,
		"gc_count":         ms.NumGC,
		"next_gc_mb":       bytesToMB(ms.NextGC),
	}

	// Best-effort: a transport hiccup shouldn't crash the host
	// process. Errors are silently dropped here; the records
	// transport already logs reconnect attempts.
	_ = c.Send(app, record)
}

// pausePercentiles holds p50/p95/p99 of GC pause durations,
// converted to microseconds for readability in dashboards.
type pausePercentiles struct {
	p50, p95, p99 int64
}

// pausePercentilesMicroseconds extracts a histogram of GC pauses
// from runtime/metrics. Falls back to MemStats.PauseNs (a ring of
// up to 256 most-recent pauses) when the runtime/metrics endpoint
// isn't available — that ring is still cheap and always populated.
func pausePercentilesMicroseconds(ms *runtime.MemStats) pausePercentiles {
	// Try runtime/metrics first — it's the canonical source.
	samples := []gormetrics.Sample{
		{Name: "/gc/pauses:seconds"},
	}
	gormetrics.Read(samples)
	if h := samples[0].Value; h.Kind() == gormetrics.KindFloat64Histogram {
		hist := h.Float64Histogram()
		if p := percentilesFromHistogram(hist, 0.5, 0.95, 0.99); p != nil {
			return pausePercentiles{
				p50: secondsToMicros(p[0]),
				p95: secondsToMicros(p[1]),
				p99: secondsToMicros(p[2]),
			}
		}
	}
	return pausePercentilesFromRing(ms)
}

func pausePercentilesFromRing(ms *runtime.MemStats) pausePercentiles {
	// MemStats.PauseNs is a 256-entry ring buffer in ns; only the
	// first min(NumGC, 256) entries are valid. Sort and pick.
	n := int(ms.NumGC)
	if n > len(ms.PauseNs) {
		n = len(ms.PauseNs)
	}
	if n == 0 {
		return pausePercentiles{}
	}
	pauses := make([]uint64, n)
	for i := 0; i < n; i++ {
		idx := (uint64(i) + uint64(ms.NumGC) + uint64(len(ms.PauseNs)) - uint64(n)) % uint64(len(ms.PauseNs))
		pauses[i] = ms.PauseNs[idx]
	}
	sort.Slice(pauses, func(i, j int) bool { return pauses[i] < pauses[j] })
	return pausePercentiles{
		p50: int64(pauses[pickRank(n, 0.5)] / 1000),
		p95: int64(pauses[pickRank(n, 0.95)] / 1000),
		p99: int64(pauses[pickRank(n, 0.99)] / 1000),
	}
}

func pickRank(n int, p float64) int {
	if n == 0 {
		return 0
	}
	r := int(p * float64(n-1))
	if r < 0 {
		r = 0
	}
	if r >= n {
		r = n - 1
	}
	return r
}

// percentilesFromHistogram returns the requested percentiles
// against a runtime/metrics float64 histogram (cumulative bucket
// counts). Returns nil if the histogram is empty or malformed.
func percentilesFromHistogram(h *gormetrics.Float64Histogram, ps ...float64) []float64 {
	if h == nil || len(h.Counts) == 0 {
		return nil
	}
	var total uint64
	for _, c := range h.Counts {
		total += c
	}
	if total == 0 {
		return nil
	}
	out := make([]float64, len(ps))
	for i, p := range ps {
		target := uint64(p * float64(total))
		var cum uint64
		for j, c := range h.Counts {
			cum += c
			if cum >= target {
				// Buckets is len(Counts)+1; pick the upper edge of
				// this bucket as the percentile estimate.
				out[i] = h.Buckets[j+1]
				break
			}
		}
	}
	return out
}

func bytesToMB(b uint64) float64 {
	return float64(b) / (1024.0 * 1024.0)
}

func secondsToMicros(s float64) int64 {
	return int64(s * 1_000_000)
}
