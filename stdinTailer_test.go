// Copyright 2016-2026 The grok_exporter Authors
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
	"github.com/jdrews/go-tailer/fswatcher"
	"os"
	"testing"
	"time"
)

// Tests the stdin tailer by swapping a pipe into os.Stdin. This pins the
// current behavior: lines are delivered on Lines(), and when stdin reaches
// EOF an error is sent on Errors() and the reader goroutine exits.
//
// Known limitation (documented, not fixed here): Close() is a no-op, so if
// stdin never reaches EOF the reader goroutine leaks for the lifetime of the
// process. The stdin tailer is intended for `cat ... | logstation`-style
// usage where the pipe's EOF is the natural end of the stream.
func TestStdinTailer(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() failed: %v", err)
	}

	savedStdin := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = savedStdin
		w.Close()
		r.Close()
	}()

	tailer := RunStdinTailer()

	if _, err := w.Write([]byte("hello\nworld\n")); err != nil {
		t.Fatalf("write to stdin pipe failed: %v", err)
	}

	line, ok := readLine(t, tailer, 5*time.Second)
	if !ok || line.Line != "hello" {
		t.Fatalf("first line = %q (ok=%v), want %q", line, ok, "hello")
	}
	line, ok = readLine(t, tailer, 5*time.Second)
	if !ok || line.Line != "world" {
		t.Fatalf("second line = %q (ok=%v), want %q", line, ok, "world")
	}

	// Closing the write end causes EOF on the reader: an error is sent on
	// the Errors() channel and the goroutine exits.
	w.Close()
	select {
	case err, open := <-tailer.Errors():
		if !open {
			t.Fatal("Errors() channel was closed without an error")
		}
		if err == nil {
			t.Fatal("expected a non-nil EOF error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the EOF error")
	}
}

func readLine(t *testing.T, tailer fswatcher.FileTailer, timeout time.Duration) (*fswatcher.Line, bool) {
	t.Helper()
	select {
	case line, open := <-tailer.Lines():
		return line, open
	case <-time.After(timeout):
		return nil, false
	}
}
