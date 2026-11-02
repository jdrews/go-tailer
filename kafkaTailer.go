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
	ctx "context"
	"fmt"
	"github.com/IBM/sarama"
	configuration "github.com/jdrews/go-tailer/config"
	"github.com/jdrews/go-tailer/fswatcher"
	"github.com/sirupsen/logrus"
	"sync"
)

type KafkaTailer struct {
	lines  chan *fswatcher.Line
	errors chan fswatcher.Error
	cancel ctx.CancelFunc
	done   chan struct{} // closed when the consumer goroutine has fully exited
}

type consumer struct {
	ready     chan bool
	lineChan  chan *fswatcher.Line
	errorChan chan fswatcher.Error
}

func (t *KafkaTailer) Lines() chan *fswatcher.Line {
	return t.lines
}

func (t *KafkaTailer) Errors() chan fswatcher.Error {
	return t.errors
}

// Close cancels the consumer, waits for the consumer goroutine to exit, and
// then closes the Lines()/Errors() channels. It is safe to call once the
// tailer has already reported a configuration error.
func (t *KafkaTailer) Close() {
	t.cancel()
	<-t.done
	close(t.lines)
	close(t.errors)
}

// RunKafkaTailer runs the kafka tailer
func RunKafkaTailer(cfg *configuration.InputConfig) fswatcher.FileTailer {
	lineChan := make(chan *fswatcher.Line)
	errorChan := make(chan fswatcher.Error)

	kctx, cancel := ctx.WithCancel(ctx.Background())
	tailer := &KafkaTailer{
		lines:  lineChan,
		errors: errorChan,
		cancel: cancel,
		done:   make(chan struct{}),
	}

	go initKafkaConsumer(tailer, kctx, cfg)

	return tailer
}

// initKafkaConsumer runs the sarama consumer group. Configuration errors are
// reported on the Errors() channel and never panic: a bad config string must
// not be able to kill the process.
func initKafkaConsumer(tailer *KafkaTailer, kctx ctx.Context, cfg *configuration.InputConfig) {
	defer close(tailer.done)

	errorChan := tailer.errors
	// Error sends are guarded by the context, following the pattern used in
	// the file tailer: if we are shutting down and the consumer is not
	// reading the Errors() channel, we must not block the shutdown.
	sendError := func(cause error, msg string) {
		select {
		case errorChan <- fswatcher.NewError(fswatcher.NotSpecified, cause, msg):
		case <-kctx.Done():
		}
	}

	version, err := sarama.ParseKafkaVersion(cfg.KafkaVersion)
	if err != nil {
		sendError(err, "[Kafka] Error parsing Kafka version")
		return
	}

	/**
	 * Construct a new Sarama configuration.
	 * The Kafka cluster version has to be defined before the consumer/producer is initialized.
	 */

	consumer := consumer{
		ready:     make(chan bool),
		lineChan:  tailer.lines,
		errorChan: errorChan,
	}

	kafkaConfig := sarama.NewConfig()
	kafkaConfig.Version = version

	switch cfg.KafkaPartitionAssignor {
	case "sticky":
		kafkaConfig.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategySticky
	case "roundrobin":
		kafkaConfig.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	case "range":
		kafkaConfig.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRange
	default:
		sendError(fmt.Errorf("unrecognized consumer group partition assignor %q", cfg.KafkaPartitionAssignor), "[Kafka] invalid configuration")
		return
	}

	if cfg.KafkaConsumeFromOldest {
		kafkaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest
	}

	/**
	 * Setup a new Sarama consumer group
	 */

	client, err := sarama.NewConsumerGroup(cfg.KafkaBrokers, cfg.KafkaConsumerGroupName, kafkaConfig)
	if err != nil {
		sendError(err, "[Kafka] Error creating client")
		return
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			// `Consume` should be called inside an infinite loop, when a
			// server-side rebalance happens, the consumer session will need to be
			// recreated to get the new claims
			if err := client.Consume(kctx, cfg.KafkaTopics, &consumer); err != nil {
				sendError(err, "[Kafka] Error from consumer")
			}
			// check if context was cancelled, signaling that the consumer should stop
			if kctx.Err() != nil {
				logrus.Infof("[Kafka] Consumer %s goroutine exiting.", cfg.KafkaConsumerGroupName)
				return
			}
			consumer.ready = make(chan bool)
		}
	}()

	// Await till the consumer has been set up. This must be interruptible: if
	// the broker is unreachable, Setup is never called and we must not block
	// forever (Close() cancels the context, which unwinds the consume loop).
	select {
	case <-consumer.ready:
		logrus.Infof("[Kafka] Consumer %s active.", cfg.KafkaConsumerGroupName)
	case <-kctx.Done():
	}

	<-kctx.Done()
	logrus.Info("[Kafka] Consumer terminating: context cancelled")

	wg.Wait()

	if err := client.Close(); err != nil {
		sendError(err, "[Kafka] Error closing client")
	}

	logrus.Info("[Kafka] Client has been closed")
}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (consumer *consumer) Setup(sarama.ConsumerGroupSession) error {
	// Mark the consumer as ready
	close(consumer.ready)
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
func (consumer *consumer) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim must start a consumer loop of ConsumerGroupClaim's Messages().
func (consumer *consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {

	for message := range claim.Messages() {
		logrus.Debugf("[Kafka] Message content: %s", string(message.Value))
		session.MarkMessage(message, "")
		consumer.lineChan <- &fswatcher.Line{Line: string(message.Value)}
	}

	return nil
}
