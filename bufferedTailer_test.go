// Copyright 2016-2020 The grok_exporter Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package go_tailer

import (
	"fmt"
	"github.com/jdrews/go-tailer/fswatcher"
	"github.com/sirupsen/logrus"
	"math/rand"
	"sync"
	"testing"
	"time"
)

const nTestLines = 10000

var log = logrus.New()

type sourceTailer struct {
	lines chan *fswatcher.Line
}

func (tail *sourceTailer) Lines() chan *fswatcher.Line {
	return tail.lines
}

func (tail *sourceTailer) Errors() chan fswatcher.Error {
	return nil
}

func (tail *sourceTailer) Close() {
	close(tail.lines)
}

// TODO: As we separated lineBuffer and the metrics, this test is now partially copy-and-paste from lineBuffer_test
func TestLineBufferSequential_withMetrics(t *testing.T) {
	src := &sourceTailer{lines: make(chan *fswatcher.Line)}
	metric := &peakLoadMetric{}
	buffered := BufferedTailerWithMetrics(src, metric, log, 0)
	for i := 1; i <= nTestLines; i++ {
		src.lines <- &fswatcher.Line{Line: fmt.Sprintf("This is line number %v.", i)}
	}
	for i := 1; i <= nTestLines; i++ {
		line := <-buffered.Lines()
		if line.Line != fmt.Sprintf("This is line number %v.", i) {
			t.Errorf("Expected 'This is line number %v', but got '%v'.", i, line)
		}
	}
	buffered.Close()
	_, stillOpen := <-buffered.Lines()
	if stillOpen {
		t.Error("Buffered tailer was not closed.")
	}
	_, stillOpen = <-src.Lines()
	if stillOpen {
		t.Error("Source tailer was not closed.")
	}
	if !metric.StartCalled() {
		t.Error("metric.Start() not called.")
	}
	// metric.Stop() is called by the producer goroutine after it observes the closed
	// source channel. buffered.Close() does not wait for that, so we poll for it.
	if !waitFor(metric.StopCalled) {
		t.Error("metric.Stop() not called.")
	}
	// The buffer is filled before the test starts reading, so the peak load should be
	// nTestLines (at most a couple of lines less, depending on how quick the consumer
	// loop started reading).
	peak := metric.PeakLoad()
	if peak < nTestLines-2 || peak > nTestLines {
		t.Errorf("peak load = %v, expected between %v and %v", peak, nTestLines-2, nTestLines)
	}
	fmt.Printf("peak load (should be between %v and %v): %v\n", nTestLines-2, nTestLines, peak)
}

// waitFor polls condition every 10ms for up to 2 seconds.
// It is used to wait for effects that happen in background goroutines
// (e.g. metric.Stop()) which are not synchronized with Close().
func waitFor(condition func() bool) bool {
	deadline := time.Now().Add(2 * time.Second)
	for {
		if condition() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TODO: As we separated lineBuffer and the metrics, this test is now partially copy-and-paste from lineBuffer_test
func TestLineBufferParallel_withMetrics(t *testing.T) {
	src := &sourceTailer{lines: make(chan *fswatcher.Line)}
	metric := &peakLoadMetric{}
	buffered := BufferedTailerWithMetrics(src, metric, log, 0)
	var wg sync.WaitGroup
	go func() {
		start := time.Now()
		for i := 1; i <= nTestLines; i++ {
			src.lines <- &fswatcher.Line{Line: fmt.Sprintf("This is line number %v.", i)}
			if rand.Int()%64 == 0 { // Sleep from time to time
				time.Sleep(10 * time.Millisecond)
			}
		}
		fmt.Printf("Producer took %v.\n", time.Since(start))
		wg.Done()
	}()
	go func() {
		start := time.Now()
		for i := 1; i <= nTestLines; i++ {
			line := <-buffered.Lines()
			if line.Line != fmt.Sprintf("This is line number %v.", i) {
				t.Errorf("Expected 'This is line number %v', but got '%v'.", i, line)
			}
			if rand.Int()%64 == 0 { // Sleep from time to time
				time.Sleep(10 * time.Millisecond)
			}
		}
		fmt.Printf("Consumer took %v.\n", time.Since(start))
		wg.Done()
	}()
	wg.Add(2)
	wg.Wait()
	buffered.Close()
	_, stillOpen := <-buffered.Lines()
	if stillOpen {
		t.Error("Buffered tailer was not closed.")
	}
	_, stillOpen = <-src.Lines()
	if stillOpen {
		t.Error("Source tailer was not closed.")
	}
	if !metric.StartCalled() {
		t.Error("metric.Start() not called.")
	}
	// metric.Stop() is called by the producer goroutine after it observes the closed
	// source channel. buffered.Close() does not wait for that, so we poll for it.
	if !waitFor(metric.StopCalled) {
		t.Error("metric.Stop() not called.")
	}
	// Should be much less than nTestLines, because consumer and producer work in parallel.
	// The bound is generous because under the race detector the consumer goroutine is
	// slowed down significantly (observed peaks of ~12% of nTestLines).
	peak := metric.PeakLoad()
	if peak >= nTestLines/4 {
		t.Errorf("peak load = %v, expected less than %v", peak, nTestLines/4)
	}
	fmt.Printf("peak load (should be much less than %v): %v\n", nTestLines, peak)
}

// peakLoadMetric is a test double for BufferLoadMetric.
// Start/Inc are called by the buffered tailer's producer goroutine and
// Dec/Stop by other goroutines, so all state is guarded by mu.
// Tests the maxLinesInBuffer > 0 path: when the buffer reaches the limit,
// its contents are dropped (metric.Set(0)) and only the newest lines survive.
// The exact set of surviving lines depends on scheduling (how many lines the
// consumer managed to pop before the first drop), so this test asserts
// scheduling-independent invariants instead of an exact sequence.
func TestBufferedTailerDropsLinesWhenBufferFull(t *testing.T) {
	const (
		nLines           = 20
		maxLinesInBuffer = 5
	)
	src := &sourceTailer{lines: make(chan *fswatcher.Line)}
	metric := &peakLoadMetric{}
	buffered := BufferedTailerWithMetrics(src, metric, log, maxLinesInBuffer)

	// Produce all lines without reading from the buffered tailer. The consumer
	// goroutine can deliver at most one line before it blocks on the unread
	// out channel, so the buffer fills up and lines are dropped.
	for i := 1; i <= nLines; i++ {
		src.lines <- &fswatcher.Line{Line: fmt.Sprintf("line %v", i)}
	}
	src.Close()

	var delivered []string
	for line := range buffered.Lines() {
		delivered = append(delivered, line.Line)
	}

	if n := metric.SetCalls(); n < 1 {
		t.Errorf("metric.Set(0) was called %v times, expected at least 1 (no lines were dropped)", n)
	}
	if len(delivered) >= nLines {
		t.Errorf("got %v lines, expected fewer than %v because lines must be dropped", len(delivered), nLines)
	}
	if len(delivered) < 1 {
		t.Fatalf("got no lines at all")
	}
	// The delivered lines must be a strictly increasing subsequence of the
	// produced lines: no reordering, no duplicates.
	for i, line := range delivered {
		var n int
		if _, err := fmt.Sscanf(line, "line %v", &n); err != nil {
			t.Fatalf("delivered line %q is not in the expected format", line)
		}
		if i > 0 && n <= parsedLine(delivered[i-1]) {
			t.Errorf("delivered lines are not strictly increasing: %v", delivered)
		}
	}
	// The last produced line is the last one in the buffer, and the buffer is
	// only cleared before a push, so it must always survive.
	if delivered[len(delivered)-1] != fmt.Sprintf("line %v", nLines) {
		t.Errorf("last delivered line = %q, want %q", delivered[len(delivered)-1], fmt.Sprintf("line %v", nLines))
	}
	// The drop logic must keep the buffer at or below the limit.
	if peak := metric.PeakLoad(); peak > maxLinesInBuffer {
		t.Errorf("peak load = %v, expected at most %v", peak, maxLinesInBuffer)
	}
	if !waitFor(metric.StopCalled) {
		t.Error("metric.Stop() not called.")
	}
}

func parsedLine(line string) int {
	var n int
	fmt.Sscanf(line, "line %v", &n)
	return n
}

type peakLoadMetric struct {
	mu          sync.Mutex
	startCalled bool
	stopCalled  bool
	peakLoad    int64
	currentLoad int64
	setCalls    int
}

func (m *peakLoadMetric) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalled = true
}

func (m *peakLoadMetric) Inc() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentLoad++
	if m.peakLoad < m.currentLoad {
		m.peakLoad = m.currentLoad
	}
}

func (m *peakLoadMetric) Dec() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentLoad--
}

func (m *peakLoadMetric) Set(value int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentLoad = value
	m.setCalls++
}

func (m *peakLoadMetric) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalled = true
}

// Accessors for the test goroutine, so reads are race-free.

func (m *peakLoadMetric) StartCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startCalled
}

func (m *peakLoadMetric) StopCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopCalled
}

func (m *peakLoadMetric) PeakLoad() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.peakLoad
}

func (m *peakLoadMetric) SetCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.setCalls
}
