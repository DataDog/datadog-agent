package kafka

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	sarama "github.com/Shopify/sarama"
	"github.com/netsampler/goflow2/transport"
	"github.com/netsampler/goflow2/utils"

	log "github.com/sirupsen/logrus"
)

type KafkaDriver struct {
	kafkaTLS            bool
	kafkaSASL           bool
	kafkaTopic          string
	kafkaSrv            string
	kafkaBrk            string
	kafkaMaxMsgBytes    int
	kafkaFlushBytes     int
	kafkaFlushFrequency time.Duration

	kafkaLogErrors bool

	kafkaHashing bool
	kafkaVersion string

	producer sarama.AsyncProducer

	q chan bool
}

func (d *KafkaDriver) Prepare() error {
	flag.BoolVar(&d.kafkaTLS, "transport.kafka.tls", false, "Use TLS to connect to Kafka")

	flag.BoolVar(&d.kafkaSASL, "transport.kafka.sasl", false, "Use SASL/PLAIN data to connect to Kafka (TLS is recommended and the environment variables KAFKA_SASL_USER and KAFKA_SASL_PASS need to be set)")
	flag.StringVar(&d.kafkaTopic, "transport.kafka.topic", "flow-messages", "Kafka topic to produce to")
	flag.StringVar(&d.kafkaSrv, "transport.kafka.srv", "", "SRV record containing a list of Kafka brokers (or use brokers)")
	flag.StringVar(&d.kafkaBrk, "transport.kafka.brokers", "127.0.0.1:9092,[::1]:9092", "Kafka brokers list separated by commas")
	flag.IntVar(&d.kafkaMaxMsgBytes, "transport.kafka.maxmsgbytes", 1000000, "Kafka max message bytes")
	flag.IntVar(&d.kafkaFlushBytes, "transport.kafka.flushbytes", int(sarama.MaxRequestSize), "Kafka flush bytes")
	flag.DurationVar(&d.kafkaFlushFrequency, "transport.kafka.flushfreq", time.Second*5, "Kafka flush frequency")

	flag.BoolVar(&d.kafkaLogErrors, "transport.kafka.log.err", false, "Log Kafka errors")
	flag.BoolVar(&d.kafkaHashing, "transport.kafka.hashing", false, "Enable partition hashing")

	//flag.StringVar(&d.kafkaKeying, "transport.kafka.key", "SamplerAddress,DstAS", "Kafka list of fields to do hashing on (partition) separated by commas")
	flag.StringVar(&d.kafkaVersion, "transport.kafka.version", "2.8.0", "Kafka version")

	return nil
}

func (d *KafkaDriver) Init(context.Context) error {
	kafkaConfigVersion, err := sarama.ParseKafkaVersion(d.kafkaVersion)
	if err != nil {
		return err
	}

	kafkaConfig := sarama.NewConfig()
	kafkaConfig.Version = kafkaConfigVersion
	kafkaConfig.Producer.Return.Successes = false
	kafkaConfig.Producer.Return.Errors = d.kafkaLogErrors
	kafkaConfig.Producer.MaxMessageBytes = d.kafkaMaxMsgBytes
	kafkaConfig.Producer.Flush.Bytes = d.kafkaFlushBytes
	kafkaConfig.Producer.Flush.Frequency = d.kafkaFlushFrequency
	if d.kafkaTLS {
		rootCAs, err := x509.SystemCertPool()
		if err != nil {
			return errors.New(fmt.Sprintf("Error initializing TLS: %v", err))
		}
		kafkaConfig.Net.TLS.Enable = true
		kafkaConfig.Net.TLS.Config = &tls.Config{RootCAs: rootCAs}
	}

	if d.kafkaHashing {
		kafkaConfig.Producer.Partitioner = sarama.NewHashPartitioner
	}

	if d.kafkaSASL {
		if !d.kafkaTLS /*&& log != nil*/ {
			log.Warn("Using SASL without TLS will transmit the authentication in plaintext!")
		}
		kafkaConfig.Net.SASL.Enable = true
		kafkaConfig.Net.SASL.User = os.Getenv("KAFKA_SASL_USER")
		kafkaConfig.Net.SASL.Password = os.Getenv("KAFKA_SASL_PASS")
		if kafkaConfig.Net.SASL.User == "" && kafkaConfig.Net.SASL.Password == "" {
			return errors.New("Kafka SASL config from environment was unsuccessful. KAFKA_SASL_USER and KAFKA_SASL_PASS need to be set.")
		} else /*if log != nil*/ {
			log.Infof("Authenticating as user '%s'...", kafkaConfig.Net.SASL.User)
		}
	}

	addrs := make([]string, 0)
	if d.kafkaSrv != "" {
		addrs, _ = utils.GetServiceAddresses(d.kafkaSrv)
	} else {
		addrs = strings.Split(d.kafkaBrk, ",")
	}

	kafkaProducer, err := sarama.NewAsyncProducer(addrs, kafkaConfig)
	if err != nil {
		return err
	}
	d.producer = kafkaProducer

	d.q = make(chan bool)

	if d.kafkaLogErrors {
		go func() {
			for {
				select {
				case msg := <-kafkaProducer.Errors():
					//if log != nil {
					log.Error(msg)
					//}
				case <-d.q:
					return
				}
			}
		}()
	}

	return err
}

func (d *KafkaDriver) Send(key, data []byte) error {
	d.producer.Input() <- &sarama.ProducerMessage{
		Topic: d.kafkaTopic,
		Key:   sarama.ByteEncoder(key),
		Value: sarama.ByteEncoder(data),
	}
	return nil
}

func (d *KafkaDriver) Close(context.Context) error {
	d.producer.Close()
	close(d.q)
	return nil
}

func init() {
	d := &KafkaDriver{}
	transport.RegisterTransportDriver("kafka", d)
}
