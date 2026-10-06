package rabbitmq

import (
	"errors"
	"fmt"
	"log"

	"github.com/dinghen/CogniGo/config"
	"github.com/streadway/amqp"
)

const (
	// RabbitMQ queues and messages must both be durable for async persistence to
	// survive a broker restart.
	durableQueue      = true
	persistentMessage = amqp.Persistent
)

// 全局connection对象
// 所有RabbitMQ都会复用该对象
var conn *amqp.Connection

// 初始化connection
func initConn() {
	c := config.GetConfig()
	mqUrl := fmt.Sprintf(
		"amqp://%s:%s@%s:%d/%s",
		c.RabbitmqUsername, c.RabbitmqPassword, c.RabbitmqHost, c.RabbitmqPort, c.RabbitmqVhost,
	)
	log.Printf("connecting to RabbitMQ at %s:%d", c.RabbitmqHost, c.RabbitmqPort)
	var err error
	conn, err = amqp.Dial(mqUrl)
	if err != nil {
		log.Fatalf("RabbitMQ connection failed: %v", err) // 输出错误并退出程序
	}
}

// RabbitMQ RabbitMQ结构体
type RabbitMQ struct {
	conn     *amqp.Connection
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
	_ = r.channel.Close()
	_ = r.conn.Close()
}

// NewWorkRabbitMQ 创建Work模式的RabbitMQ实例
func NewWorkRabbitMQ(queue string) *RabbitMQ {
	// new rabbitmq
	rabbitmq := NewRabbitMQ("", queue)

	// get connection
	if conn == nil {
		initConn()
	}
	rabbitmq.conn = conn

	// get channel
	var err error
	rabbitmq.channel, err = rabbitmq.conn.Channel()
	if err != nil {
		panic(err.Error())
	}

	return rabbitmq
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
