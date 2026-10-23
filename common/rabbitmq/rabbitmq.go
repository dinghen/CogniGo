package rabbitmq

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"syscall"
	"time"

	"github.com/dinghen/CogniGo/config"
	"github.com/streadway/amqp"
)

const (
	// RabbitMQ queues and messages must both be durable for async persistence to
	// survive a broker restart.
	durableQueue      = true
	persistentMessage = amqp.Persistent
	// RabbitMQ may report a healthy container before its AMQP listener is
	// accepting connections. Keep startup retries bounded, but long enough to
	// cover that broker initialization window in the local demo.
	maxDialAttempts  = 12
	initialDialDelay = 500 * time.Millisecond
	maxDialDelay     = 2 * time.Second
)

type rabbitMQConnection interface {
	Channel() (*amqp.Channel, error)
	Close() error
}

// 全局connection对象
// 所有RabbitMQ都会复用该对象
var conn rabbitMQConnection

// 初始化connection
func initConn() error {
	if conn != nil {
		return nil
	}

	c := config.GetConfig()
	mqUrl := fmt.Sprintf(
		"amqp://%s:%s@%s:%d/%s",
		c.RabbitmqUsername, c.RabbitmqPassword, c.RabbitmqHost, c.RabbitmqPort, c.RabbitmqVhost,
	)
	endpoint := fmt.Sprintf("%s:%d", c.RabbitmqHost, c.RabbitmqPort)
	log.Printf("connecting to RabbitMQ at %s", endpoint)
	newConn, err := dialWithRetry(endpoint, func() (rabbitMQConnection, error) {
		connection, err := amqp.Dial(mqUrl)
		if connection == nil {
			return nil, err
		}
		return connection, err
	}, time.Sleep)
	if err != nil {
		return err
	}
	conn = newConn
	return nil
}

func dialWithRetry(endpoint string, dial func() (rabbitMQConnection, error), wait func(time.Duration)) (rabbitMQConnection, error) {
	for attempt := 1; attempt <= maxDialAttempts; attempt++ {
		connection, err := dial()
		if err == nil {
			if connection == nil {
				return nil, fmt.Errorf("RabbitMQ dial to %s returned an empty connection", endpoint)
			}
			return connection, nil
		}

		if connection != nil {
			if closeErr := connection.Close(); closeErr != nil {
				log.Printf("failed to close partial RabbitMQ connection to %s: %v", endpoint, closeErr)
			}
		}

		if !isRetryableDialError(err) || attempt == maxDialAttempts {
			return nil, fmt.Errorf("RabbitMQ connection to %s failed after %d attempt(s): %w", endpoint, attempt, err)
		}

		delay := dialRetryDelay(attempt)
		log.Printf("RabbitMQ connection to %s failed on attempt %d/%d (%T); retrying in %s: %v", endpoint, attempt, maxDialAttempts, err, delay, err)
		wait(delay)
	}

	return nil, fmt.Errorf("RabbitMQ connection to %s failed", endpoint)
}

func dialRetryDelay(failedAttempt int) time.Duration {
	delay := initialDialDelay
	for i := 1; i < failedAttempt && delay < maxDialDelay; i++ {
		if delay > maxDialDelay/2 {
			return maxDialDelay
		}
		delay *= 2
	}
	if delay > maxDialDelay {
		return maxDialDelay
	}
	return delay
}

func isRetryableDialError(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	// streadway/amqp wraps frame read failures without preserving the original
	// error. Retry only the exact EOF reasons, not arbitrary frame errors.
	var amqpErr *amqp.Error
	if errors.As(err, &amqpErr) && amqpErr.Code == amqp.FrameError {
		if amqpErr.Reason == io.EOF.Error() || amqpErr.Reason == io.ErrUnexpectedEOF.Error() {
			return true
		}
	}

	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNABORTED) ||
		errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}

	var networkErr net.Error
	return errors.As(err, &networkErr) && (networkErr.Timeout() || networkErr.Temporary())
}

// RabbitMQ RabbitMQ结构体
type RabbitMQ struct {
	conn     rabbitMQConnection
	channel  *amqp.Channel
	Exchange string
	Key      string
}

// NewRabbitMQ 创建RabbitMQ对象
func NewRabbitMQ(exchange string, key string) *RabbitMQ {
	return &RabbitMQ{Exchange: exchange, Key: key}
}

// Destroy 断开 channel 和 connection
func (r *RabbitMQ) Destroy() {
	if r.channel != nil {
		_ = r.channel.Close()
	}
	if r.conn != nil {
		_ = r.conn.Close()
	}
}

// NewWorkRabbitMQ 创建Work模式的RabbitMQ实例
func NewWorkRabbitMQ(queue string) (*RabbitMQ, error) {
	// new rabbitmq
	rabbitmq := NewRabbitMQ("", queue)

	// get connection
	if conn == nil {
		if err := initConn(); err != nil {
			return nil, err
		}
	}
	rabbitmq.conn = conn

	// get channel
	var err error
	rabbitmq.channel, err = rabbitmq.conn.Channel()
	if err != nil {
		_ = rabbitmq.conn.Close()
		conn = nil
		return nil, fmt.Errorf("open RabbitMQ channel for queue %q: %w", queue, err)
	}

	return rabbitmq, nil
}

// Publish 发送消息
func (r *RabbitMQ) Publish(message []byte) error {
	// 创建队列（不存在时）
	// 使用默认交换机的情况下，queue即为key
	_, err := r.channel.QueueDeclare(r.Key, durableQueue, false, false, false, nil)
	if err != nil {
		return err
	}

	// 调用 channel 发送消息到队列
	return r.channel.Publish(r.Exchange, r.Key, false, false,
		amqp.Publishing{
			ContentType:  "text/plain",
			DeliveryMode: persistentMessage,
			Body:         message,
		},
	)
}

type deliveryActions interface {
	Ack(multiple bool) error
	Nack(multiple, requeue bool) error
	Reject(requeue bool) error
}

// PermanentDeliveryError marks a message that cannot succeed on retry, such
// as malformed JSON. Rejecting it prevents a poison message from blocking the
// persistence queue indefinitely.
type PermanentDeliveryError struct {
	Err error
}

func (e *PermanentDeliveryError) Error() string { return e.Err.Error() }
func (e *PermanentDeliveryError) Unwrap() error { return e.Err }

// settleDelivery acknowledges a message only after the handler succeeds. A
// failed handler is requeued so a transient database or dependency error does
// not silently discard the message.
func settleDelivery(delivery deliveryActions, handlerErr error) error {
	if handlerErr == nil {
		return delivery.Ack(false)
	}
	var permanentErr *PermanentDeliveryError
	if errors.As(handlerErr, &permanentErr) {
		return delivery.Reject(false)
	}
	return delivery.Nack(false, true)
}

// Consume 消费者
// handle: 消息的消费业务函数，用于消费消息
func (r *RabbitMQ) Consume(handle func(msg *amqp.Delivery) error) {
	// 创建队列
	q, err := r.channel.QueueDeclare(r.Key, durableQueue, false, false, false, nil)
	if err != nil {
		panic(err)
	}

	// 接收消息
	msgs, err := r.channel.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		panic(err)
	}

	// 处理消息
	for msg := range msgs {
		handlerErr := handle(&msg)
		if handlerErr != nil {
			log.Printf("RabbitMQ message handler failed: %v", handlerErr)
		}
		if err := settleDelivery(&msg, handlerErr); err != nil {
			log.Printf("RabbitMQ message settlement failed: %v", err)
		}
	}
}
