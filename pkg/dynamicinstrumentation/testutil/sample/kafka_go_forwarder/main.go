package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/datastreams"

	"github.com/Shopify/sarama"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	ddsarama "gopkg.in/DataDog/dd-trace-go.v1/contrib/Shopify/sarama"
	ddkafka "gopkg.in/DataDog/dd-trace-go.v1/contrib/confluentinc/confluent-kafka-go/kafka.v2"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	producer  = "PRODUCER"
	consumer  = "CONSUMER"
	forwarder = "FORWARDER"
)

var bootStrapServers = os.Getenv("KAFKA_BOOTSTRAP_SERVERS")

func main() {
	role := os.Getenv("APP_ROLE")
	tracer.Start()
	defer tracer.Stop()
	switch role {
	case producer:
		startSaramaProducer()
	case consumer:
		startConfluentConsumer()
	case forwarder:
		startForwarder()
	default:
		log.Printf("Unknow role %s", role)
	}
}

func startSaramaProducer() {
	config := sarama.NewConfig()
	producer, err := sarama.NewAsyncProducer([]string{bootStrapServers}, config)
	if err != nil {
		panic(err)
	}
	producer = ddsarama.WrapAsyncProducer(config, producer, ddsarama.WithDataStreams())

	defer func() {
		if err := producer.Close(); err != nil {
			panic(err)
		}
	}()

	for {
		message := &sarama.ProducerMessage{
			Topic: "go-topic-1",
			Value: sarama.StringEncoder("hello"),
		}

		producer.Input() <- message
		fmt.Println("Sent message")

		time.Sleep(time.Second)
	}
}

func startForwarder() {
	// forwarder consumes using Sarama consumer, and produces using confluent producer
	config := sarama.NewConfig()
	config.Consumer.Return.Errors = true

	producer, err := ddkafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": bootStrapServers,
	}, ddkafka.WithDataStreams())
	if err != nil {
		panic(err)
	}
	defer producer.Close()

	consumer, err := sarama.NewConsumer([]string{bootStrapServers}, config)
	if err != nil {
		panic(err)
	}
	consumer = ddsarama.WrapConsumer(consumer, ddsarama.WithDataStreams())

	defer func() {
		if err := consumer.Close(); err != nil {
			panic(err)
		}
	}()

	partitionConsumer, err := consumer.ConsumePartition("go-topic-1", 0, sarama.OffsetNewest)
	if err != nil {
		panic(err)
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case message := <-partitionConsumer.Messages():
			forwardMessage(message, producer)
		case err := <-partitionConsumer.Errors():
			fmt.Printf("Error: %s\n", err.Error())
		case <-signals:
			return
		}
	}
}

func forwardMessage(message *sarama.ConsumerMessage, producer *ddkafka.Producer) {
	fmt.Printf("forwarding message: %s\n", string(message.Value))
	topic := "go-topic-2"
	ctx := context.Background()
	ctx = datastreams.ExtractFromBase64Carrier(ctx, ddsarama.NewConsumerMessageCarrier(message))
	msg := &kafka.Message{TopicPartition: kafka.TopicPartition{Topic: &topic}, Value: message.Value}
	datastreams.InjectToBase64Carrier(ctx, ddkafka.NewMessageCarrier(msg))
	if err := producer.Produce(msg, nil); err != nil {
		fmt.Printf("Error producing to Kafka %v", err)
	}
}

func startConfluentConsumer() {
	c, err := ddkafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": bootStrapServers,
		"group.id":          "confluent-consumer",
	}, ddkafka.WithDataStreams())
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			panic(err)
		}
	}()
	err = c.SubscribeTopics([]string{"go-topic-2", "python-topic-1", "nodejs-topic-1"}, func(consumer *kafka.Consumer, event kafka.Event) error {
		return nil
	})
	if err != nil {
		panic(err)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	for {
		if m, err := c.ReadMessage(time.Second); err == nil {
			fmt.Println("Reading message", string(m.Value))
		}
		select {
		case <-signals:
			return
		default:
			continue
		}
	}
}
