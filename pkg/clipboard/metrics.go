// Package clipboard metrics implementation
//
// This file provides a metrics interface for monitoring clipboard operations,
// allowing integration with various metrics backends like Prometheus, StatsD, etc.

package clipboard

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// MetricsCollector defines the interface for collecting clipboard operation metrics.
type MetricsCollector interface {
	// Operation metrics
	RecordOperation(op string, duration time.Duration, err error)
	RecordSize(op string, size int)

	// Error metrics
	RecordError(op string, err error)
	RecordTimeout(op string)
	RecordRateLimitHit(op string)

	// State metrics
	RecordWatcherCount(count int)
	RecordSequenceNumber(seq uint64)

	// Get current metrics snapshot
	GetMetrics() MetricsSnapshot
}

// MetricsSnapshot represents a point-in-time view of metrics.
type MetricsSnapshot struct {
	Operations      map[string]*OperationMetrics
	Errors          map[string]uint64
	Timeouts        map[string]uint64
	RateLimitHits   map[string]uint64
	WatcherCount    int
	SequenceNumber  uint64
	CollectionStart time.Time
	CollectionEnd   time.Time
}

// OperationMetrics tracks metrics for a specific operation type.
type OperationMetrics struct {
	Count      uint64
	TotalTime  time.Duration
	MinTime    time.Duration
	MaxTime    time.Duration
	AvgTime    time.Duration
	TotalBytes uint64
	MinBytes   uint64
	MaxBytes   uint64
	ErrorCount uint64
}

// DefaultMetricsCollector provides a basic in-memory metrics implementation.
type DefaultMetricsCollector struct {
	mu              sync.RWMutex
	operations      map[string]*operationStats
	errors          map[string]*atomic.Uint64
	timeouts        map[string]*atomic.Uint64
	rateLimitHits   map[string]*atomic.Uint64
	watcherCount    atomic.Int32
	sequenceNumber  atomic.Uint64
	collectionStart time.Time
}

type operationStats struct {
	count      atomic.Uint64
	totalTime  atomic.Int64
	minTime    atomic.Int64
	maxTime    atomic.Int64
	totalBytes atomic.Uint64
	minBytes   atomic.Uint64
	maxBytes   atomic.Uint64
	errorCount atomic.Uint64
}

// NewDefaultMetricsCollector creates a new metrics collector.
func NewDefaultMetricsCollector() *DefaultMetricsCollector {
	return &DefaultMetricsCollector{
		operations:      make(map[string]*operationStats),
		errors:          make(map[string]*atomic.Uint64),
		timeouts:        make(map[string]*atomic.Uint64),
		rateLimitHits:   make(map[string]*atomic.Uint64),
		collectionStart: time.Now(),
	}
}

// RecordOperation records metrics for a clipboard operation.
func (m *DefaultMetricsCollector) RecordOperation(op string, duration time.Duration, err error) {
	m.mu.Lock()
	stats, ok := m.operations[op]
	if !ok {
		stats = &operationStats{}
		stats.minTime.Store(int64(duration))
		m.operations[op] = stats
	}
	m.mu.Unlock()

	stats.count.Add(1)
	stats.totalTime.Add(int64(duration))

	// Update min/max times
	for {
		min := stats.minTime.Load()
		if int64(duration) >= min && min != 0 {
			break
		}
		if stats.minTime.CompareAndSwap(min, int64(duration)) {
			break
		}
	}

	for {
		max := stats.maxTime.Load()
		if int64(duration) <= max {
			break
		}
		if stats.maxTime.CompareAndSwap(max, int64(duration)) {
			break
		}
	}

	if err != nil {
		stats.errorCount.Add(1)
	}
}

// RecordSize records the size of data in a clipboard operation.
func (m *DefaultMetricsCollector) RecordSize(op string, size int) {
	// Ensure size is non-negative before conversion
	if size < 0 {
		return
	}

	sizeUint64 := uint64(size)

	m.mu.Lock()
	stats, ok := m.operations[op]
	if !ok {
		stats = &operationStats{}
		stats.minBytes.Store(sizeUint64)
		m.operations[op] = stats
	}
	m.mu.Unlock()

	stats.totalBytes.Add(sizeUint64)

	// Update min/max sizes
	for {
		min := stats.minBytes.Load()
		if sizeUint64 >= min && min != 0 {
			break
		}
		if stats.minBytes.CompareAndSwap(min, sizeUint64) {
			break
		}
	}

	for {
		max := stats.maxBytes.Load()
		if sizeUint64 <= max {
			break
		}
		if stats.maxBytes.CompareAndSwap(max, sizeUint64) {
			break
		}
	}
}

// RecordError records an error occurrence.
func (m *DefaultMetricsCollector) RecordError(op string, err error) {
	if err == nil {
		return
	}

	key := op + "_" + categorizeError(err)

	m.mu.Lock()
	counter, ok := m.errors[key]
	if !ok {
		counter = &atomic.Uint64{}
		m.errors[key] = counter
	}
	m.mu.Unlock()

	counter.Add(1)
}

// RecordTimeout records a timeout occurrence.
func (m *DefaultMetricsCollector) RecordTimeout(op string) {
	m.mu.Lock()
	counter, ok := m.timeouts[op]
	if !ok {
		counter = &atomic.Uint64{}
		m.timeouts[op] = counter
	}
	m.mu.Unlock()

	counter.Add(1)
}

// RecordRateLimitHit records when rate limit is hit.
func (m *DefaultMetricsCollector) RecordRateLimitHit(op string) {
	m.mu.Lock()
	counter, ok := m.rateLimitHits[op]
	if !ok {
		counter = &atomic.Uint64{}
		m.rateLimitHits[op] = counter
	}
	m.mu.Unlock()

	counter.Add(1)
}

// RecordWatcherCount records the current number of watchers.
func (m *DefaultMetricsCollector) RecordWatcherCount(count int) {
	// Ensure count fits in int32 bounds
	if count < math.MinInt32 || count > math.MaxInt32 {
		// Clamp to int32 bounds
		if count < math.MinInt32 {
			count = math.MinInt32
		} else if count > math.MaxInt32 {
			count = math.MaxInt32
		}
	}
	m.watcherCount.Store(int32(count)) //nolint:gosec // bounds already checked above
}

// RecordSequenceNumber records the current sequence number.
func (m *DefaultMetricsCollector) RecordSequenceNumber(seq uint64) {
	m.sequenceNumber.Store(seq)
}

// GetMetrics returns a snapshot of current metrics.
func (m *DefaultMetricsCollector) GetMetrics() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := MetricsSnapshot{
		Operations:      make(map[string]*OperationMetrics),
		Errors:          make(map[string]uint64),
		Timeouts:        make(map[string]uint64),
		RateLimitHits:   make(map[string]uint64),
		WatcherCount:    int(m.watcherCount.Load()),
		SequenceNumber:  m.sequenceNumber.Load(),
		CollectionStart: m.collectionStart,
		CollectionEnd:   time.Now(),
	}

	// Copy operation metrics
	for op, stats := range m.operations {
		count := stats.count.Load()
		if count == 0 {
			continue
		}

		totalTime := time.Duration(stats.totalTime.Load())
		// Safe division - count is guaranteed to be > 0 here, and we ensure it fits in int64
		var avgTime time.Duration
		if count <= math.MaxInt64 {
			avgTime = totalTime / time.Duration(count)
		} else {
			// For extremely large counts, use floating point calculation
			avgTime = time.Duration(float64(totalTime) / float64(count))
		}

		snapshot.Operations[op] = &OperationMetrics{
			Count:      count,
			TotalTime:  totalTime,
			MinTime:    time.Duration(stats.minTime.Load()),
			MaxTime:    time.Duration(stats.maxTime.Load()),
			AvgTime:    avgTime,
			TotalBytes: stats.totalBytes.Load(),
			MinBytes:   stats.minBytes.Load(),
			MaxBytes:   stats.maxBytes.Load(),
			ErrorCount: stats.errorCount.Load(),
		}
	}

	// Copy error counts
	for key, counter := range m.errors {
		snapshot.Errors[key] = counter.Load()
	}

	// Copy timeout counts
	for op, counter := range m.timeouts {
		snapshot.Timeouts[op] = counter.Load()
	}

	// Copy rate limit hit counts
	for op, counter := range m.rateLimitHits {
		snapshot.RateLimitHits[op] = counter.Load()
	}

	return snapshot
}

// categorizeError categorizes errors for metrics grouping.
func categorizeError(err error) string {
	switch err {
	case nil:
		return "none"
	case ErrNotSupported:
		return "not_supported"
	case ErrContentTooLarge:
		return "content_too_large"
	case context.DeadlineExceeded:
		return "timeout"
	case context.Canceled:
		return "cancelled"
	default:
		// Generic categorization based on error message
		errStr := err.Error()
		switch {
		case contains(errStr, "timeout"):
			return "timeout"
		case contains(errStr, "permission"):
			return "permission"
		case contains(errStr, "not found"):
			return "not_found"
		case contains(errStr, "rate limit"):
			return "rate_limit"
		default:
			return "other"
		}
	}
}

// NoOpMetricsCollector is a no-op implementation for when metrics are disabled.
type NoOpMetricsCollector struct{}

func (n *NoOpMetricsCollector) RecordOperation(_ string, _ time.Duration, _ error) {}
func (n *NoOpMetricsCollector) RecordSize(_ string, _ int)                               {}
func (n *NoOpMetricsCollector) RecordError(_ string, _ error)                             {}
func (n *NoOpMetricsCollector) RecordTimeout(_ string)                                      {}
func (n *NoOpMetricsCollector) RecordRateLimitHit(_ string)                                 {}
func (n *NoOpMetricsCollector) RecordWatcherCount(_ int)                                 {}
func (n *NoOpMetricsCollector) RecordSequenceNumber(_ uint64)                              {}
func (n *NoOpMetricsCollector) GetMetrics() MetricsSnapshot                                  { return MetricsSnapshot{} }
