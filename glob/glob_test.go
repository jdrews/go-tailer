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

package glob

import (
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name     string
		pattern  string
		want     string
		wantErr  bool
	}{
		{
			name:    "plain file name",
			pattern: filepath.Join(dir, "logfile.log"),
			want:    filepath.Join(dir, "logfile.log"),
		},
		{
			name:    "wildcard in file name",
			pattern: filepath.Join(dir, "*.log"),
			want:    filepath.Join(dir, "*.log"),
		},
		{
			name:    "character class in file name",
			pattern: filepath.Join(dir, "log[0-9].log"),
			want:    filepath.Join(dir, "log[0-9].log"),
		},
		{
			name:     "wildcard in directory path is rejected",
			pattern:  filepath.Join(dir, "*", "sub.log"),
			wantErr:  true,
		},
		{
			name:     "character class in directory path is rejected",
			pattern:  filepath.Join(dir, "a[b]", "sub.log"),
			wantErr:  true,
		},
		{
			name:     "invalid pattern is rejected",
			pattern:  filepath.Join(dir, "bad["),
			wantErr:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g, err := Parse(tc.pattern)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) expected error, got %q", tc.pattern, g)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.pattern, err)
			}
			if string(g) != tc.want {
				t.Fatalf("Parse(%q) = %q, want %q", tc.pattern, g, tc.want)
			}
		})
	}
}

func TestParseRelativePatternBecomesAbsolute(t *testing.T) {
	g, err := Parse("relative/logfile.log")
	if err != nil {
		t.Fatalf("Parse(relative/logfile.log) unexpected error: %v", err)
	}
	if !filepath.IsAbs(string(g)) {
		t.Fatalf("Parse(relative/logfile.log) = %q, want absolute path", g)
	}
}

func TestGlobMatch(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		{"exact match", filepath.Join(dir, "a.log"), filepath.Join(dir, "a.log"), true},
		{"different file", filepath.Join(dir, "a.log"), filepath.Join(dir, "b.log"), false},
		{"star matches file name", filepath.Join(dir, "*.log"), filepath.Join(dir, "a.log"), true},
		{"star does not match other extension", filepath.Join(dir, "*.log"), filepath.Join(dir, "a.txt"), false},
		{"question mark matches one char", filepath.Join(dir, "a?.log"), filepath.Join(dir, "a1.log"), true},
		{"question mark does not match two chars", filepath.Join(dir, "a?.log"), filepath.Join(dir, "aab.log"), false},
		{"character class matches", filepath.Join(dir, "a[0-9].log"), filepath.Join(dir, "a1.log"), true},
		{"character class does not match", filepath.Join(dir, "a[0-9].log"), filepath.Join(dir, "ax.log"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := Glob(tc.pattern)
			if got := g.Match(tc.path); got != tc.want {
				t.Fatalf("Match(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
			}
		})
	}
}

func TestGlobDir(t *testing.T) {
	tests := []struct {
		glob Glob
		want string
	}{
		{Glob(filepath.Join("a", "b", "c.log")), filepath.Join("a", "b")},
		{Glob("c.log"), "."},
		{Glob(filepath.Join("a", "*.log")), filepath.Join("a")},
	}
	for _, tc := range tests {
		if got := tc.glob.Dir(); got != tc.want {
			t.Fatalf("Dir() of %q = %q, want %q", tc.glob, got, tc.want)
		}
	}
}
