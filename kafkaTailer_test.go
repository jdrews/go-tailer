// Copyright 2020-2026 The grok_exporter Authors
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
	configuration "github.com/jdrews/go-tailer/config"
	"github.com/jdrews/go-tailer/fswatcher"
	"github.com/IBM/sarama"
	logStd "log"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"
)

func init() {
	// Keep the test output readable: sarama logs connection attempts and
	// retries at high volume.
	sarama.Logger = logStd.New(io.Discard, "", 0)
}

// expectError waits up to timeout for an error on the Errors() channel.
func expectError(t *testing.T, tailer fswatcher.FileTailer, timeout time.Duration) (fswatcher.Error, bool) {
	t.Helper()
	select {
	case err, open := <-tailer.Errors():
		if !open {
			t.Fatal("Errors() channel was closed without an error")
		}
		return err, true
	case <-time.After(timeout):
		return nil, false
	}
}

// assertShutdown verifies that Close() closes both channels and that the
// consumer goroutine terminates (no goroutine leak).
func assertShutdown(t *testing.T, tailer fswatcher.FileTailer, nGoroutinesBefore int) {
	t.Helper()
	tailer.Close()
	if _, open := <-tailer.Lines(); open {
		t.Error("Lines() channel was not closed after Close()")
	}
	if _, open := <-tailer.Errors(); open {
		t.Error("Errors() channel was not closed after Close()")
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && runtime.NumGoroutine() > nGoroutinesBefore {
		time.Sleep(50 * time.Millisecond)
	}
	if n := runtime.NumGoroutine() - nGoroutinesBefore; n > 0 {
		t.Errorf("%v goroutine(s) did not shut down after Close()", n)
	}
}

// A bad Kafka version must be reported on the Errors() channel, not with a
// panic. (A panic in the consumer goroutine would crash the whole test
// process; reaching the assertions below is the proof that it did not.)
func TestKafkaTailerBadVersion(t *testing.T) {
	nBefore := runtime.NumGoroutine()
	cfg := &configuration.InputConfig{
		Type:                   "kafka",
		KafkaVersion:           "not-a-version",
		KafkaBrokers:           []string{"127.0.0.1:1"},
		KafkaTopics:            []string{"test-topic"},
		KafkaConsumerGroupName: "go-tailer-test",
		KafkaPartitionAssignor: "range",
	}
	tailer := RunKafkaTailer(cfg)

	err, ok := expectError(t, tailer, 10*time.Second)
	if !ok {
		t.Fatal("timed out waiting for the bad-version error")
	}
	if err == nil || !strings.Contains(err.Cause().Error(), "invalid version") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertShutdown(t, tailer, nBefore)
}

// An unrecognized partition assignor must be reported with a real cause
// (the old code passed a nil error), and the consumer must not start.
func TestKafkaTailerUnrecognizedAssignor(t *testing.T) {
	nBefore := runtime.NumGoroutine()
	cfg := &configuration.InputConfig{
		Type:                   "kafka",
		KafkaVersion:           "2.0.0",
		KafkaBrokers:           []string{"127.0.0.1:1"},
		KafkaTopics:            []string{"test-topic"},
		KafkaConsumerGroupName: "go-tailer-test",
		KafkaPartitionAssignor: "bogus",
	}
	tailer := RunKafkaTailer(cfg)

	err, ok := expectError(t, tailer, 10*time.Second)
	if !ok {
		t.Fatal("timed out waiting for the assignor error")
	}
	if err == nil || err.Cause() == nil {
		t.Fatalf("expected a non-nil cause, got %v", err)
	}
	if !strings.Contains(err.Cause().Error(), "bogus") {
		t.Fatalf("cause does not mention the bad assignor: %v", err.Cause())
	}
	assertShutdown(t, tailer, nBefore)
}

// An unreachable broker must be reported on the Errors() channel (the old
// code would have panicked on a nil client), and the tailer must shut down
// cleanly. 127.0.0.1:1 is refused immediately, so this stays fast; the
// timeout is generous so CI machines cannot flake it.
func TestKafkaTailerUnreachableBroker(t *testing.T) {
	nBefore := runtime.NumGoroutine()
	cfg := &configuration.InputConfig{
		Type:                   "kafka",
		KafkaVersion:           "2.0.0",
		KafkaBrokers:           []string{"127.0.0.1:1"},
		KafkaTopics:            []string{"test-topic"},
		KafkaConsumerGroupName: "go-tailer-test",
		KafkaPartitionAssignor: "range",
	}
	tailer := RunKafkaTailer(cfg)

	err, ok := expectError(t, tailer, 60*time.Second)
	if !ok {
		t.Fatal("timed out waiting for the broker error")
	}
	if err == nil {
		t.Fatal("expected a non-nil error")
	}
	assertShutdown(t, tailer, nBefore)
}
