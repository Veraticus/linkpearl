package clipboard

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultMetricsCollector_Operations(t *testing.T) {
	collector := NewDefaultMetricsCollector()

	// Record operations
	collector.RecordOperation("read", 100*time.Millisecond, nil)
	collector.RecordOperation("read", 200*time.Millisecond, nil)
	collector.RecordOperation("read", 150*time.Millisecond, errors.New("read error"))
	collector.RecordOperation("write", 50*time.Millisecond, nil)

	snapshot := collector.GetMetrics()

	// Check read metrics
	readMetrics := snapshot.Operations["read"]
	if readMetrics == nil {
		t.Fatal("Expected read metrics")
	}
	if readMetrics.Count != 3 {
		t.Errorf("Expected 3 read operations, got %d", readMetrics.Count)
	}
	if readMetrics.ErrorCount != 1 {
		t.Errorf("Expected 1 error, got %d", readMetrics.ErrorCount)
	}
	if readMetrics.TotalTime != 450*time.Millisecond {
		t.Errorf("Expected total time 450ms, got %v", readMetrics.TotalTime)
	}
	if readMetrics.MinTime != 100*time.Millisecond {
		t.Errorf("Expected min time 100ms, got %v", readMetrics.MinTime)
	}
	if readMetrics.MaxTime != 200*time.Millisecond {
		t.Errorf("Expected max time 200ms, got %v", readMetrics.MaxTime)
	}
	if readMetrics.AvgTime != 150*time.Millisecond {
		t.Errorf("Expected avg time 150ms, got %v", readMetrics.AvgTime)
	}

	// Check write metrics
	writeMetrics := snapshot.Operations["write"]
	if writeMetrics == nil {
		t.Fatal("Expected write metrics")
	}
	if writeMetrics.Count != 1 {
		t.Errorf("Expected 1 write operation, got %d", writeMetrics.Count)
	}
}

func TestDefaultMetricsCollector_Sizes(t *testing.T) {
	collector := NewDefaultMetricsCollector()

	// Record sizes
	collector.RecordSize("read", 1000)
	collector.RecordSize("read", 2000)
	collector.RecordSize("read", 1500)
	collector.RecordSize("write", 500)

	// Need to record operations to initialize the stats
	collector.RecordOperation("read", time.Millisecond, nil)
	collector.RecordOperation("write", time.Millisecond, nil)

	snapshot := collector.GetMetrics()

	// Check read size metrics
	readMetrics := snapshot.Operations["read"]
	if readMetrics == nil {
		t.Fatal("Expected read metrics")
	}
	if readMetrics.TotalBytes != 4500 {
		t.Errorf("Expected total bytes 4500, got %d", readMetrics.TotalBytes)
	}
	if readMetrics.MinBytes != 1000 {
		t.Errorf("Expected min bytes 1000, got %d", readMetrics.MinBytes)
	}
	if readMetrics.MaxBytes != 2000 {
		t.Errorf("Expected max bytes 2000, got %d", readMetrics.MaxBytes)
	}
}

func TestDefaultMetricsCollector_Errors(t *testing.T) {
	collector := NewDefaultMetricsCollector()

	// Record various errors
	collector.RecordError("read", ErrNotSupported)
	collector.RecordError("read", ErrContentTooLarge)
	collector.RecordError("read", context.DeadlineExceeded)
	collector.RecordError("write", errors.New("permission denied"))
	collector.RecordError("write", errors.New("disk full"))

	snapshot := collector.GetMetrics()

	// Check error categorization
	if snapshot.Errors["read_not_supported"] != 1 {
		t.Errorf("Expected 1 not_supported error, got %d", snapshot.Errors["read_not_supported"])
	}
	if snapshot.Errors["read_content_too_large"] != 1 {
		t.Errorf("Expected 1 content_too_large error, got %d", snapshot.Errors["read_content_too_large"])
	}
	if snapshot.Errors["read_timeout"] != 1 {
		t.Errorf("Expected 1 timeout error, got %d", snapshot.Errors["read_timeout"])
	}
	if snapshot.Errors["write_permission"] != 1 {
		t.Errorf("Expected 1 permission error, got %d", snapshot.Errors["write_permission"])
	}
}

func TestDefaultMetricsCollector_TimeoutsAndRateLimits(t *testing.T) {
	collector := NewDefaultMetricsCollector()

	// Record timeouts
	collector.RecordTimeout("read")
	collector.RecordTimeout("read")
	collector.RecordTimeout("write")

	// Record rate limit hits
	collector.RecordRateLimitHit("read")
	collector.RecordRateLimitHit("write")
	collector.RecordRateLimitHit("write")

	snapshot := collector.GetMetrics()

	if snapshot.Timeouts["read"] != 2 {
		t.Errorf("Expected 2 read timeouts, got %d", snapshot.Timeouts["read"])
	}
	if snapshot.Timeouts["write"] != 1 {
		t.Errorf("Expected 1 write timeout, got %d", snapshot.Timeouts["write"])
	}
	if snapshot.RateLimitHits["read"] != 1 {
		t.Errorf("Expected 1 read rate limit hit, got %d", snapshot.RateLimitHits["read"])
	}
	if snapshot.RateLimitHits["write"] != 2 {
		t.Errorf("Expected 2 write rate limit hits, got %d", snapshot.RateLimitHits["write"])
	}
}

func TestDefaultMetricsCollector_State(t *testing.T) {
	collector := NewDefaultMetricsCollector()

	// Record state metrics
	collector.RecordWatcherCount(5)
	collector.RecordSequenceNumber(12345)

	snapshot := collector.GetMetrics()

	if snapshot.WatcherCount != 5 {
		t.Errorf("Expected watcher count 5, got %d", snapshot.WatcherCount)
	}
	if snapshot.SequenceNumber != 12345 {
		t.Errorf("Expected sequence number 12345, got %d", snapshot.SequenceNumber)
	}
}

func TestDefaultMetricsCollector_ConcurrentAccess(t *testing.T) {
	collector := NewDefaultMetricsCollector()

	// Run concurrent operations
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				// Record operations with errors for every 10th operation
				var err error
				if j%10 == 0 {
					err = errors.New("test error")
				}
				collector.RecordOperation("read", time.Duration(j)*time.Millisecond, err)
				collector.RecordSize("read", j*100)
				if err != nil {
					collector.RecordError("read", err)
				}
			}
			done <- true
		}(i)
	}

	// Wait for completion
	for i := 0; i < 10; i++ {
		<-done
	}

	snapshot := collector.GetMetrics()

	// Verify metrics
	readMetrics := snapshot.Operations["read"]
	if readMetrics == nil {
		t.Fatal("Expected read metrics")
	}
	if readMetrics.Count != 1000 {
		t.Errorf("Expected 1000 operations, got %d", readMetrics.Count)
	}
	if readMetrics.ErrorCount != 100 {
		t.Errorf("Expected 100 errors, got %d", readMetrics.ErrorCount)
	}
}

func TestCategorizeError(t *testing.T) {
	tests := []struct {
		err      error
		expected string
	}{
		{nil, "none"},
		{ErrNotSupported, "not_supported"},
		{ErrContentTooLarge, "content_too_large"},
		{context.DeadlineExceeded, "timeout"},
		{context.Canceled, "cancelled"},
		{errors.New("operation timeout"), "timeout"},
		{errors.New("permission denied"), "permission"},
		{errors.New("file not found"), "not_found"},
		{errors.New("rate limit exceeded"), "rate_limit"},
		{errors.New("unknown error"), "other"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := categorizeError(tt.err)
			if got != tt.expected {
				t.Errorf("categorizeError(%v) = %q, want %q", tt.err, got, tt.expected)
			}
		})
	}
}

func TestNoOpMetricsCollector(t *testing.T) {
	collector := &NoOpMetricsCollector{}

	// All operations should be no-ops and not panic
	collector.RecordOperation("read", time.Second, nil)
	collector.RecordSize("read", 1000)
	collector.RecordError("read", errors.New("test"))
	collector.RecordTimeout("read")
	collector.RecordRateLimitHit("read")
	collector.RecordWatcherCount(5)
	collector.RecordSequenceNumber(100)

	snapshot := collector.GetMetrics()

	// Snapshot should be empty
	if len(snapshot.Operations) != 0 {
		t.Error("Expected empty operations map")
	}
	if len(snapshot.Errors) != 0 {
		t.Error("Expected empty errors map")
	}
}
