// Copyright 2020 The grok_exporter Authors
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

package config

import (
	"github.com/jdrews/go-tailer/glob"
	"time"
)

type InputConfig struct {
	Type                       string `yaml:",omitempty"`
	PathsAndGlobs              `yaml:",inline"`
	FailOnMissingLogfileString string        `yaml:"fail_on_missing_logfile,omitempty"` // cannot use bool directly, because yaml.v2 doesn't support true as default value.
	FailOnMissingLogfile       bool          `yaml:"-"`
	Readall                    bool          `yaml:",omitempty"`
	PollInterval               time.Duration `yaml:"poll_interval,omitempty"` // implicitly parsed with time.ParseDuration()
	MaxLinesInBuffer           int           `yaml:"max_lines_in_buffer,omitempty"`
	WebhookPath                string        `yaml:"webhook_path,omitempty"`
	WebhookFormat              string        `yaml:"webhook_format,omitempty"`
	WebhookJsonSelector        string        `yaml:"webhook_json_selector,omitempty"`
	WebhookTextBulkSeparator   string        `yaml:"webhook_text_bulk_separator,omitempty"`
	KafkaVersion               string        `yaml:"kafka_version,omitempty"`
	KafkaBrokers               []string      `yaml:"kafka_brokers,omitempty"`
	KafkaTopics                []string      `yaml:"kafka_topics,omitempty"`
	KafkaPartitionAssignor     string        `yaml:"kafka_partition_assignor,omitempty"`
	KafkaConsumerGroupName     string        `yaml:"kafka_consumer_group_name,omitempty"`
	KafkaConsumeFromOldest     bool          `yaml:"kafka_consume_from_oldest,omitempty"`
}

type PathsAndGlobs struct {
	Path  string      `yaml:",omitempty"`
	Paths []string    `yaml:",omitempty"`
	Globs []glob.Glob `yaml:"-"`
}
