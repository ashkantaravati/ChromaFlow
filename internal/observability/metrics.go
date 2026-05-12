package observability

import (
	"bufio"
	"chromaflow/internal/queue"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var renderBuckets = []float64{1, 2.5, 5, 10, 15, 30, 60, 120}
var httpBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type Metrics struct {
	mu sync.RWMutex

	jobsSubmitted uint64
	jobsRejected  map[string]uint64
	jobsFinished  map[string]uint64

	renderCount   map[string]uint64
	renderSum     map[string]float64
	renderBuckets map[string][]uint64
	pdfBytes      uint64

	activeWorkers int64

	httpRequests map[httpLabel]uint64
	httpCount    map[httpLabel]uint64
	httpSum      map[httpLabel]float64
	httpBuckets  map[httpLabel][]uint64

	queueStats   func() (depth int, capacity int)
	storageStats func() StorageStats
}

type httpLabel struct {
	method string
	path   string
	code   string
}

type StorageStats struct {
	Total       int
	ByStatus    map[queue.JobStatus]int
	PDFBytes    int
	OldestJobAt time.Time
}

func NewMetrics() *Metrics {
	return &Metrics{
		jobsRejected:  make(map[string]uint64),
		jobsFinished:  make(map[string]uint64),
		renderCount:   make(map[string]uint64),
		renderSum:     make(map[string]float64),
		renderBuckets: make(map[string][]uint64),
		httpRequests:  make(map[httpLabel]uint64),
		httpCount:     make(map[httpLabel]uint64),
		httpSum:       make(map[httpLabel]float64),
		httpBuckets:   make(map[httpLabel][]uint64),
	}
}

func (m *Metrics) SetQueueStats(fn func() (depth int, capacity int)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queueStats = fn
}

func (m *Metrics) SetStorageStats(fn func() StorageStats) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storageStats = fn
}

func (m *Metrics) JobSubmitted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobsSubmitted++
}

func (m *Metrics) JobRejected(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobsRejected[reason]++
}

func (m *Metrics) WorkerStarted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeWorkers++
}

func (m *Metrics) WorkerStopped() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activeWorkers--
}

func (m *Metrics) RenderFinished(status string, duration time.Duration, bytes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	seconds := duration.Seconds()
	m.jobsFinished[status]++
	m.renderCount[status]++
	m.renderSum[status] += seconds
	if _, ok := m.renderBuckets[status]; !ok {
		m.renderBuckets[status] = make([]uint64, len(renderBuckets))
	}
	for i, upper := range renderBuckets {
		if seconds <= upper {
			m.renderBuckets[status][i]++
		}
	}
	if bytes > 0 {
		m.pdfBytes += uint64(bytes)
	}
}

func (m *Metrics) RecordHTTP(method, path string, code int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	label := httpLabel{method: method, path: path, code: strconv.Itoa(code)}
	m.httpRequests[label]++
	m.httpCount[label]++
	m.httpSum[label] += duration.Seconds()
	if _, ok := m.httpBuckets[label]; !ok {
		m.httpBuckets[label] = make([]uint64, len(httpBuckets))
	}
	for i, upper := range httpBuckets {
		if duration.Seconds() <= upper {
			m.httpBuckets[label][i]++
		}
	}
}

func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		m.WritePrometheus(w)
	})
}

func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusResponseWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rw, r)
		path := r.Pattern
		if path == "" {
			path = r.URL.Path
		}
		m.RecordHTTP(r.Method, path, rw.code, time.Since(start))
	})
}

func (m *Metrics) WritePrometheus(w http.ResponseWriter) {
	m.mu.RLock()
	jobsSubmitted := m.jobsSubmitted
	jobsRejected := cloneStringUint64(m.jobsRejected)
	jobsFinished := cloneStringUint64(m.jobsFinished)
	renderCount := cloneStringUint64(m.renderCount)
	renderSum := cloneStringFloat64(m.renderSum)
	renderBucketValues := cloneStringBuckets(m.renderBuckets)
	pdfBytes := m.pdfBytes
	activeWorkers := m.activeWorkers
	httpRequests := cloneHTTPUint64(m.httpRequests)
	httpCount := cloneHTTPUint64(m.httpCount)
	httpSum := cloneHTTPFloat64(m.httpSum)
	httpBucketValues := cloneHTTPBuckets(m.httpBuckets)
	queueStats := m.queueStats
	storageStats := m.storageStats
	m.mu.RUnlock()

	writeHelp(w, "chromaflow_jobs_submitted_total", "Total PDF jobs accepted by ChromaFlow.", "counter")
	fmt.Fprintf(w, "chromaflow_jobs_submitted_total %d\n", jobsSubmitted)

	writeHelp(w, "chromaflow_jobs_rejected_total", "Total PDF jobs rejected before enqueueing.", "counter")
	for _, reason := range sortedStringKeys(jobsRejected) {
		fmt.Fprintf(w, "chromaflow_jobs_rejected_total{reason=%q} %d\n", reason, jobsRejected[reason])
	}

	writeHelp(w, "chromaflow_jobs_finished_total", "Total PDF render jobs finished by terminal status.", "counter")
	for _, status := range sortedStringKeys(jobsFinished) {
		fmt.Fprintf(w, "chromaflow_jobs_finished_total{status=%q} %d\n", status, jobsFinished[status])
	}

	writeHelp(w, "chromaflow_pdf_render_duration_seconds", "PDF render duration by terminal status.", "histogram")
	for _, status := range sortedStringKeys(renderCount) {
		for i, upper := range renderBuckets {
			fmt.Fprintf(w, "chromaflow_pdf_render_duration_seconds_bucket{status=%q,le=%q} %d\n", status, formatFloat(upper), renderBucketValues[status][i])
		}
		fmt.Fprintf(w, "chromaflow_pdf_render_duration_seconds_bucket{status=%q,le=\"+Inf\"} %d\n", status, renderCount[status])
		fmt.Fprintf(w, "chromaflow_pdf_render_duration_seconds_sum{status=%q} %s\n", status, formatFloat(renderSum[status]))
		fmt.Fprintf(w, "chromaflow_pdf_render_duration_seconds_count{status=%q} %d\n", status, renderCount[status])
	}

	writeHelp(w, "chromaflow_pdf_bytes_total", "Total completed PDF bytes generated.", "counter")
	fmt.Fprintf(w, "chromaflow_pdf_bytes_total %d\n", pdfBytes)

	writeHelp(w, "chromaflow_active_workers", "Currently running worker goroutines.", "gauge")
	fmt.Fprintf(w, "chromaflow_active_workers %d\n", activeWorkers)

	if queueStats != nil {
		depth, capacity := queueStats()
		writeHelp(w, "chromaflow_queue_depth", "Current queue depth.", "gauge")
		fmt.Fprintf(w, "chromaflow_queue_depth %d\n", depth)
		writeHelp(w, "chromaflow_queue_capacity", "Configured queue capacity.", "gauge")
		fmt.Fprintf(w, "chromaflow_queue_capacity %d\n", capacity)
	}

	if storageStats != nil {
		stats := storageStats()
		writeHelp(w, "chromaflow_jobs_in_storage", "Current jobs tracked by status.", "gauge")
		for _, status := range sortedJobStatusKeys(stats.ByStatus) {
			fmt.Fprintf(w, "chromaflow_jobs_in_storage{status=%q} %d\n", status, stats.ByStatus[status])
		}
		writeHelp(w, "chromaflow_storage_jobs_total", "Current total jobs tracked.", "gauge")
		fmt.Fprintf(w, "chromaflow_storage_jobs_total %d\n", stats.Total)
		writeHelp(w, "chromaflow_storage_pdf_bytes", "Current completed PDF bytes tracked in storage metadata.", "gauge")
		fmt.Fprintf(w, "chromaflow_storage_pdf_bytes %d\n", stats.PDFBytes)
		if !stats.OldestJobAt.IsZero() {
			writeHelp(w, "chromaflow_storage_oldest_job_timestamp_seconds", "Unix timestamp for oldest job tracked in memory.", "gauge")
			fmt.Fprintf(w, "chromaflow_storage_oldest_job_timestamp_seconds %d\n", stats.OldestJobAt.Unix())
		}
	}

	writeHelp(w, "chromaflow_http_requests_total", "Total HTTP requests by method, route, and status code.", "counter")
	for _, label := range sortedHTTPKeys(httpRequests) {
		fmt.Fprintf(w, "chromaflow_http_requests_total{method=%q,path=%q,code=%q} %d\n", label.method, label.path, label.code, httpRequests[label])
	}

	writeHelp(w, "chromaflow_http_request_duration_seconds", "HTTP request duration by method, route, and status code.", "histogram")
	for _, label := range sortedHTTPKeys(httpCount) {
		for i, upper := range httpBuckets {
			fmt.Fprintf(w, "chromaflow_http_request_duration_seconds_bucket{method=%q,path=%q,code=%q,le=%q} %d\n", label.method, label.path, label.code, formatFloat(upper), httpBucketValues[label][i])
		}
		fmt.Fprintf(w, "chromaflow_http_request_duration_seconds_bucket{method=%q,path=%q,code=%q,le=\"+Inf\"} %d\n", label.method, label.path, label.code, httpCount[label])
		fmt.Fprintf(w, "chromaflow_http_request_duration_seconds_sum{method=%q,path=%q,code=%q} %s\n", label.method, label.path, label.code, formatFloat(httpSum[label]))
		fmt.Fprintf(w, "chromaflow_http_request_duration_seconds_count{method=%q,path=%q,code=%q} %d\n", label.method, label.path, label.code, httpCount[label])
	}
}

type statusResponseWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (w *statusResponseWriter) Flush() {
	flusher, ok := w.ResponseWriter.(http.Flusher)
	if ok {
		flusher.Flush()
	}
}

func (w *statusResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func writeHelp(w http.ResponseWriter, name, help, metricType string) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", name, escapeHelp(help), name, metricType)
}

func escapeHelp(help string) string {
	return strings.ReplaceAll(strings.ReplaceAll(help, "\\", "\\\\"), "\n", "\\n")
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func cloneStringUint64(in map[string]uint64) map[string]uint64 {
	out := make(map[string]uint64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStringFloat64(in map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStringBuckets(in map[string][]uint64) map[string][]uint64 {
	out := make(map[string][]uint64, len(in))
	for k, v := range in {
		out[k] = append([]uint64(nil), v...)
	}
	return out
}

func cloneHTTPUint64(in map[httpLabel]uint64) map[httpLabel]uint64 {
	out := make(map[httpLabel]uint64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneHTTPFloat64(in map[httpLabel]float64) map[httpLabel]float64 {
	out := make(map[httpLabel]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneHTTPBuckets(in map[httpLabel][]uint64) map[httpLabel][]uint64 {
	out := make(map[httpLabel][]uint64, len(in))
	for k, v := range in {
		out[k] = append([]uint64(nil), v...)
	}
	return out
}

func sortedStringKeys[V any](in map[string]V) []string {
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedJobStatusKeys(in map[queue.JobStatus]int) []queue.JobStatus {
	keys := make([]queue.JobStatus, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func sortedHTTPKeys(in map[httpLabel]uint64) []httpLabel {
	keys := make([]httpLabel, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].method != keys[j].method {
			return keys[i].method < keys[j].method
		}
		if keys[i].path != keys[j].path {
			return keys[i].path < keys[j].path
		}
		return keys[i].code < keys[j].code
	})
	return keys
}
