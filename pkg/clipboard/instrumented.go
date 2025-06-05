// Package clipboard instrumented implementation
//
// This file provides an instrumented clipboard wrapper that collects metrics
// for all clipboard operations.
package clipboard

import (
	"context"
	"time"
)

// InstrumentedClipboard wraps a clipboard implementation with metrics collection.
type InstrumentedClipboard struct {
	clipboard Clipboard
	metrics   MetricsCollector
}

// NewInstrumentedClipboard creates a new instrumented clipboard.
func NewInstrumentedClipboard(clipboard Clipboard, metrics MetricsCollector) *InstrumentedClipboard {
	if metrics == nil {
		metrics = &NoOpMetricsCollector{}
	}
	return &InstrumentedClipboard{
		clipboard: clipboard,
		metrics:   metrics,
	}
}

// Read returns the current clipboard contents with metrics.
func (ic *InstrumentedClipboard) Read() (string, error) {
	start := time.Now()

	content, err := ic.clipboard.Read()

	duration := time.Since(start)
	ic.metrics.RecordOperation("read", duration, err)

	if err == nil {
		ic.metrics.RecordSize("read", len(content))
	} else {
		ic.metrics.RecordError("read", err)
		if err == context.DeadlineExceeded {
			ic.metrics.RecordTimeout("read")
		}
	}

	return content, err
}

// Write sets the clipboard contents with metrics.
func (ic *InstrumentedClipboard) Write(content string) error {
	start := time.Now()

	ic.metrics.RecordSize("write", len(content))

	err := ic.clipboard.Write(content)

	duration := time.Since(start)
	ic.metrics.RecordOperation("write", duration, err)

	if err != nil {
		ic.metrics.RecordError("write", err)
		if err == context.DeadlineExceeded {
			ic.metrics.RecordTimeout("write")
		}
	}

	return err
}

// Watch monitors clipboard changes with metrics.
func (ic *InstrumentedClipboard) Watch(ctx context.Context) <-chan struct{} {
	// Create wrapped channel
	wrappedCh := make(chan struct{}, 10)

	// Get the underlying channel
	underlyingCh := ic.clipboard.Watch(ctx)

	// Track watcher count
	ic.incrementWatcherCount()

	// Forward notifications with metrics
	go func() {
		defer close(wrappedCh)
		defer ic.decrementWatcherCount()

		for {
			select {
			case <-ctx.Done():
				return
			case notification, ok := <-underlyingCh:
				if !ok {
					return
				}

				// Record sequence number from state
				state := ic.clipboard.GetState()
				ic.metrics.RecordSequenceNumber(state.SequenceNumber)

				// Forward notification
				select {
				case wrappedCh <- notification:
				case <-ctx.Done():
					return
				default:
					// Channel full, skip
				}
			}
		}
	}()

	return wrappedCh
}

// GetState returns current state information.
func (ic *InstrumentedClipboard) GetState() State {
	return ic.clipboard.GetState()
}

// GetMetrics returns the metrics collector for external access.
func (ic *InstrumentedClipboard) GetMetrics() MetricsCollector {
	return ic.metrics
}

// incrementWatcherCount increments the watcher count metric.
func (ic *InstrumentedClipboard) incrementWatcherCount() {
	// This is a simplified implementation - in production you'd track actual count
	// For now, just increment by 1
	ic.metrics.RecordWatcherCount(1)
}

// decrementWatcherCount decrements the watcher count metric.
func (ic *InstrumentedClipboard) decrementWatcherCount() {
	// This is a simplified implementation - in production you'd track actual count
	// For now, just set to 0
	ic.metrics.RecordWatcherCount(0)
}
