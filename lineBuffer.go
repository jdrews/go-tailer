// Copyright 2019-2020 The grok_exporter Authors
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
	"container/list"
	"github.com/jdrews/go-tailer/fswatcher"
	"io"
	logFatal "log"
	"sync"
)

// lineBuffer is a thread safe queue for *fswatcher.Line.
type lineBuffer interface {
	Push(line *fswatcher.Line)
	BlockingPop() *fswatcher.Line // can be interrupted by calling Close()
	Len() int
	io.Closer // will interrupt BlockingPop()
	Clear()
}

func NewLineBuffer() lineBuffer {
	return &lineBufferImpl{
		buffer: list.New(),
		lock:   sync.NewCond(&sync.Mutex{}),
		closed: false,
	}
}

type lineBufferImpl struct {
	buffer *list.List
	lock   *sync.Cond
	closed bool
}

func (b *lineBufferImpl) Push(line *fswatcher.Line) {
	b.lock.L.Lock()
	defer b.lock.L.Unlock()
	if !b.closed {
		b.buffer.PushBack(line)
		b.lock.Signal()
	}
}

// Interrupted by Close(), returns nil when Close() is called.
func (b *lineBufferImpl) BlockingPop() *fswatcher.Line {
	b.lock.L.Lock()
	defer b.lock.L.Unlock()
	if !b.closed {
		for b.buffer.Len() == 0 && !b.closed {
			b.lock.Wait()
		}
		if !b.closed {
			first := b.buffer.Front()
			b.buffer.Remove(first)
			switch line := first.Value.(type) {
			case *fswatcher.Line:
				return line
			default:
				// this cannot happen
				logFatal.Fatal("unexpected type in tailer b.buffer")
			}
		}
	}
	return nil
}

func (b *lineBufferImpl) Close() error {
	b.lock.L.Lock()
	defer b.lock.L.Unlock()
	if !b.closed {
		b.closed = true
		b.lock.Signal()
	}
	return nil
}

func (b *lineBufferImpl) Len() int {
	b.lock.L.Lock()
	defer b.lock.L.Unlock()
	return b.buffer.Len()
}

func (b *lineBufferImpl) Clear() {
	b.lock.L.Lock()
	defer b.lock.L.Unlock()
	b.buffer = list.New()
}
