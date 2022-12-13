package amqp

import (
	"log"

	"github.com/streadway/amqp"
)

// Here we set the way error messages are displayed in the terminal.
func logError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func Send() {
	// Here we connect to RabbitMQ or send a message if there are any errors connecting.
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	logError(err, "Failed to connect to RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	logError(err, "Failed to open a channel")
	defer ch.Close()

	// We create a Queue to send the message to.
	q, err := ch.QueueDeclare(
		"test-queue", // name
		false,        // durable
		false,        // delete when unused
		false,        // exclusive
		false,        // no-wait
		nil,          // arguments
	)
	logError(err, "Failed to declare a queue")

	// We set the payload for the message.
	body := "This is a test body"
	err = ch.Publish(
		"",     // exchange
		q.Name, // routing key
		false,  // mandatory
		false,  // immediate
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte(body),
		})
	// If there is an error publishing the message, a log will be displayed in the terminal.
	logError(err, "Failed to publish a message")
	log.Printf(" [x] Congrats, sending message: %s", body)
}
