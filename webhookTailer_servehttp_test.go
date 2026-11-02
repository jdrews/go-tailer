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
	"bytes"
	configuration "github.com/jdrews/go-tailer/config"
	"net/http"
	"net/http/httptest"
	"testing"
)

// installWebhookTailer resets the package-global singleton and installs a
// fresh webhook tailer with the given config. Each subtest gets its own
// Lines()/Errors() channels, so cases cannot interfere with each other.
func installWebhookTailer(cfg *configuration.InputConfig) *WebhookTailer {
	webhookTailerSingleton = nil
	return InitWebhookTailer(cfg).(*WebhookTailer)
}

// Tests the HTTP layer of the webhook tailer (ServeHTTP), which is not covered
// by the WebhookProcessBody tests.
func TestWebhookServeHTTP(t *testing.T) {
	savedSingleton := webhookTailerSingleton
	defer func() { webhookTailerSingleton = savedSingleton }()

	// ServeHTTP sends on the unbuffered Lines()/Errors() channels, so it
	// blocks until the test reads. Start it in a goroutine; the test must
	// read the expected channel value BEFORE joining on done, or both
	// goroutines block forever.
	startServe := func(tailer *WebhookTailer, req *http.Request) (*httptest.ResponseRecorder, <-chan struct{}) {
		rec := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			tailer.ServeHTTP(rec, req)
			close(done)
		}()
		return rec, done
	}

	t.Run("singleton", func(t *testing.T) {
		cfg := &configuration.InputConfig{Type: "webhook", WebhookFormat: "text_single"}
		tailer := InitWebhookTailer(cfg)
		// A second call must return the same instance, and WebhookHandler()
		// must expose it as the http.Handler.
		if second := InitWebhookTailer(cfg); second != tailer {
			t.Fatal("InitWebhookTailer did not return the singleton on the second call")
		}
		// WebhookHandler() returns http.Handler and tailer is
		// fswatcher.FileTailer, so compare via interface{} (both hold the
		// same *WebhookTailer pointer).
		if handler := interface{}(WebhookHandler()); handler != interface{}(tailer) {
			t.Fatal("WebhookHandler() did not return the singleton")
		}
	})

	t.Run("text_single", func(t *testing.T) {
		cfg := &configuration.InputConfig{Type: "webhook", WebhookPath: "/webhook", WebhookFormat: "text_single"}
		tailer := installWebhookTailer(cfg)

		rec, done := startServe(tailer, httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString("hello world")))
		line, ok := <-tailer.Lines() // unblocks ServeHTTP
		<-done
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %v, want %v (body: %v)", rec.Code, http.StatusOK, rec.Body.String())
		}
		if !ok {
			t.Fatal("Lines() channel was closed")
		}
		if line.Line != "hello world" {
			t.Fatalf("line = %q, want %q", line.Line, "hello world")
		}
	})

	t.Run("json_single_with_extra", func(t *testing.T) {
		cfg := &configuration.InputConfig{
			Type:                "webhook",
			WebhookPath:         "/webhook",
			WebhookFormat:       "json_single",
			WebhookJsonSelector: ".message",
		}
		tailer := installWebhookTailer(cfg)

		rec, done := startServe(tailer, httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(`{"message": "json line", "host": "1.1.1.1"}`)))
		line, ok := <-tailer.Lines() // unblocks ServeHTTP
		<-done
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %v, want %v (body: %v)", rec.Code, http.StatusOK, rec.Body.String())
		}
		if !ok {
			t.Fatal("Lines() channel was closed")
		}
		if line.Line != "json line" {
			t.Fatalf("line = %q, want %q", line.Line, "json line")
		}
		extra, ok := line.Extra.(map[string]interface{})
		if !ok {
			t.Fatalf("Extra = %T, want map[string]interface{}", line.Extra)
		}
		if extra["host"] != "1.1.1.1" {
			t.Errorf("Extra[host] = %v, want 1.1.1.1", extra["host"])
		}
	})

	// A nil request body (not an empty one: httptest always sets a body) must
	// result in a 400 and an error on the Errors() channel.
	t.Run("nil_body", func(t *testing.T) {
		cfg := &configuration.InputConfig{Type: "webhook", WebhookPath: "/webhook", WebhookFormat: "text_single"}
		tailer := installWebhookTailer(cfg)

		rec, done := startServe(tailer, &http.Request{})
		err := <-tailer.Errors() // unblocks ServeHTTP
		<-done
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %v, want %v", rec.Code, http.StatusBadRequest)
		}
		if err == nil {
			t.Fatal("expected an error on the Errors() channel")
		}
	})
}
