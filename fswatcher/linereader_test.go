// Copyright 2026 The grok_exporter Authors
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

package fswatcher

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// scriptedReader returns the configured (data, err) pairs one per Read() call,
// in order. After the last pair it returns (0, io.EOF).
type scriptedReader struct {
	data []string
	errs []error
	idx  int
}

func (r *scriptedReader) Read(p []byte) (int, error) {
	if r.idx >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.idx])
	err := r.errs[r.idx]
	r.idx++
	return n, err
}

// chunkedReader returns at most size bytes per Read() call.
func chunkedReader(data string, size int) *scriptedReader {
	var (
		chunks []string
		errs   []error
	)
	for len(data) > 0 {
		if len(data) > size {
			chunks = append(chunks, data[:size])
			data = data[size:]
		} else {
			chunks = append(chunks, data)
			data = ""
		}
	}
	for range chunks {
		errs = append(errs, nil)
	}
	return &scriptedReader{data: chunks, errs: errs}
}

// expectLines reads lines until eof and compares them with expected.
func expectLines(t *testing.T, r *lineReader, file io.Reader, expected ...string) {
	t.Helper()
	i := 0
	for {
		line, eof, err := r.ReadLine(file)
		if err != nil {
			t.Fatalf("ReadLine() returned error: %v", err)
		}
		if eof {
			if i != len(expected) {
				t.Fatalf("expected %v lines, got %v: %v", len(expected), i, expected[:i])
			}
			return
		}
		if i >= len(expected) {
			t.Fatalf("unexpected extra line %q", line)
		}
		if line != expected[i] {
			t.Fatalf("line %d: expected %q, got %q", i, expected[i], line)
		}
		i++
	}
}

func TestLineReaderBasicLines(t *testing.T) {
	r := NewLineReader()
	// The third line is empty: an empty line in the file must be reported as
	// an empty line, not skipped.
	expectLines(t, r, strings.NewReader("line one\nline two\n\n"), "line one", "line two", "")
}

func TestLineReaderChunkedReads(t *testing.T) {
	// A line that only completes after several Read() calls.
	r := NewLineReader()
	expectLines(t, r, chunkedReader("hello world\n", 3), "hello world")

	// Multiple lines, one byte at a time (the smallest possible chunks).
	r = NewLineReader()
	expectLines(t, r, chunkedReader("ab\ncd\nef\n", 1), "ab", "cd", "ef")

	// Data left over from a previous line in the internal buffer must be
	// carried over to the next line.
	r = NewLineReader()
	expectLines(t, r, chunkedReader("xyz\nab\ncd\n", 4), "xyz", "ab", "cd")
}

func TestLineReaderBufferBoundary(t *testing.T) {
	// The internal read buffer is 512 bytes. Lines of exactly 512 bytes
	// (511 chars + newline), 513 bytes, and 1536 bytes must all come out
	// intact, which exercises a line that ends exactly on a buffer boundary
	// and lines that span two and three buffer reads.
	line511 := strings.Repeat("a", 511)  // +\n = 512 bytes
	line512 := strings.Repeat("b", 512)  // +\n = 513 bytes
	line1535 := strings.Repeat("c", 1535) // +\n = 1536 bytes
	r := NewLineReader()
	expectLines(t, r, strings.NewReader(line511+"\n"+line512+"\n"+line1535+"\n"), line511, line512, line1535)
}

func TestLineReaderWindowsLineEndings(t *testing.T) {
	r := NewLineReader()
	// \r\n is stripped to \n.
	expectLines(t, r, strings.NewReader("abc\r\ndef\n"), "abc", "def")

	// A \r that is not immediately before the line ending is preserved.
	r = NewLineReader()
	expectLines(t, r, strings.NewReader("a\rb\n"), "a\rb")

	// \r\n\r\n is a line plus an empty line.
	r = NewLineReader()
	expectLines(t, r, strings.NewReader("abc\r\n\r\n"), "abc", "")
}

func TestLineReaderEOF(t *testing.T) {
	// A line terminated by \n, then EOF.
	r := NewLineReader()
	expectLines(t, r, strings.NewReader("abc\n"), "abc")

	// Current behavior (pinned by this test): a final line without a
	// terminating \n is discarded at EOF. ReadLine() returns ("", true, nil)
	// and the partial line is never delivered.
	r = NewLineReader()
	expectLines(t, r, strings.NewReader("abc"))

	// Same for a partial line after a complete one.
	r = NewLineReader()
	expectLines(t, r, strings.NewReader("abc\ndef"), "abc")
}

func TestLineReaderClear(t *testing.T) {
	// After the first line, the buffer holds the partial line "ab".
	// Clear() must discard it, so the next line is "cd", not "abcd".
	withClear := NewLineReader()
	reader := chunkedReader("xyz\nabcd\n", 6)
	line, eof, err := withClear.ReadLine(reader)
	if err != nil || eof || line != "xyz" {
		t.Fatalf("first line: expected %q, got %q (eof=%v, err=%v)", "xyz", line, eof, err)
	}
	withClear.Clear()
	expectLines(t, withClear, reader, "cd")

	// Control: without Clear(), the partial line is kept and the next
	// line is "abcd".
	withoutClear := NewLineReader()
	reader = chunkedReader("xyz\nabcd\n", 6)
	line, eof, err = withoutClear.ReadLine(reader)
	if err != nil || eof || line != "xyz" {
		t.Fatalf("first line: expected %q, got %q (eof=%v, err=%v)", "xyz", line, eof, err)
	}
	expectLines(t, withoutClear, reader, "abcd")
}

func TestLineReaderReadError(t *testing.T) {
	testErr := errors.New("read failed")

	// A read error with no data is propagated.
	r := NewLineReader()
	_, _, err := r.ReadLine(&scriptedReader{data: []string{""}, errs: []error{testErr}})
	if err != testErr {
		t.Fatalf("expected error %v, got %v", testErr, err)
	}

	// A read error that returns data without a newline is propagated
	// (the partial data is consumed but no line is delivered).
	r = NewLineReader()
	_, _, err = r.ReadLine(&scriptedReader{data: []string{"abc"}, errs: []error{testErr}})
	if err != testErr {
		t.Fatalf("expected error %v, got %v", testErr, err)
	}

	// Bytes containing a newline are processed before the error is
	// considered (the io.Reader contract), so the line is delivered and
	// the error is not reported for it.
	r = NewLineReader()
	line, eof, err := r.ReadLine(&scriptedReader{data: []string{"abc\n"}, errs: []error{testErr}})
	if err != nil || eof || line != "abc" {
		t.Fatalf("expected line %q, got %q (eof=%v, err=%v)", "abc", line, eof, err)
	}
}
