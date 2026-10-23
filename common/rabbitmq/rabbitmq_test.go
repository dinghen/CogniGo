package rabbitmq

import (
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/streadway/amqp"
)

type fakeRabbitMQConnection struct {
	closeCalls int
}

func (f *fakeRabbitMQConnection) Channel() (*amqp.Channel, error) {
	return nil, nil
}

func (f *fakeRabbitMQConnection) Close() error {
	f.closeCalls++
	return nil
}

func TestDialWithRetryRetriesEOFThenSucceeds(t *testing.T) {
	var attempts int
	var waits []time.Duration
	connection := &fakeRabbitMQConnection{}

	got, err := dialWithRetry("rabbitmq:5672", func() (rabbitMQConnection, error) {
		attempts++
		if attempts == 1 {
			return nil, io.EOF
		}
		return connection, nil
	}, func(delay time.Duration) {
		waits = append(waits, delay)
	})
	if err != nil {
		t.Fatalf("dialWithRetry returned an error: %v", err)
	}
	if got != connection {
		t.Fatalf("dialWithRetry returned %v, want %v", got, connection)
	}
	if attempts != 2 {
		t.Fatalf("expected two dial attempts, got %d", attempts)
	}
	if len(waits) != 1 || waits[0] != initialDialDelay {
		t.Fatalf("expected one initial backoff of %s, got %v", initialDialDelay, waits)
	}
}

func TestDialWithRetryClosesPartialConnectionAndPreservesFinalError(t *testing.T) {
	var attempts int
	var waits []time.Duration
	partial := &fakeRabbitMQConnection{}
	dialErr := fmt.Errorf("startup handshake: %w", io.ErrUnexpectedEOF)

	got, err := dialWithRetry("rabbitmq:5672", func() (rabbitMQConnection, error) {
		attempts++
		return partial, dialErr
	}, func(delay time.Duration) {
		waits = append(waits, delay)
	})
	if got != nil {
		t.Fatalf("expected no connection after exhausted retries, got %v", got)
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected final error to wrap %v, got %v", io.ErrUnexpectedEOF, err)
	}
	if attempts != maxDialAttempts {
		t.Fatalf("expected %d dial attempts, got %d", maxDialAttempts, attempts)
	}
	if len(waits) != maxDialAttempts-1 {
		t.Fatalf("expected %d waits, got %d", maxDialAttempts-1, len(waits))
	}
	if partial.closeCalls != maxDialAttempts {
		t.Fatalf("expected every partial connection to be closed, got %d closes", partial.closeCalls)
	}
	if waits[len(waits)-1] != maxDialDelay {
		t.Fatalf("expected final backoff to be capped at %s, got %s", maxDialDelay, waits[len(waits)-1])
	}
}

func TestDialWithRetryDoesNotRetryAuthenticationErrors(t *testing.T) {
	var attempts int
	var waits []time.Duration

	got, err := dialWithRetry("rabbitmq:5672", func() (rabbitMQConnection, error) {
		attempts++
		return nil, amqp.ErrCredentials
	}, func(delay time.Duration) {
		waits = append(waits, delay)
	})
	if got != nil {
		t.Fatalf("expected no connection, got %v", got)
	}
	if !errors.Is(err, amqp.ErrCredentials) {
		t.Fatalf("expected authentication error to be preserved, got %v", err)
	}
	if attempts != 1 {
		t.Fatalf("expected one attempt for authentication error, got %d", attempts)
	}
	if len(waits) != 0 {
		t.Fatalf("expected no backoff for authentication error, got %v", waits)
	}
}

func TestIsRetryableDialError(t *testing.T) {
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "eof", err: io.EOF, want: true},
		{name: "wrapped eof", err: fmt.Errorf("wrapped: %w", io.ErrUnexpectedEOF), want: true},
		{name: "amqp frame eof", err: &amqp.Error{Code: amqp.FrameError, Reason: io.EOF.Error()}, want: true},
		{name: "connection refused", err: refused, want: true},
		{name: "credentials", err: amqp.ErrCredentials, want: false},
		{name: "invalid frame", err: &amqp.Error{Code: amqp.FrameError, Reason: "invalid frame"}, want: false},
		{name: "configuration", err: errors.New("invalid AMQP URI"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isRetryableDialError(test.err); got != test.want {
				t.Fatalf("isRetryableDialError(%v) = %t, want %t", test.err, got, test.want)
			}
		})
	}
}

type fakeDeliveryActions struct {
	ackCalls      int
	nackCalls     int
	rejectCalls   int
	ackMultiple   bool
	nackMultiple  bool
	nackRequeue   bool
	rejectRequeue bool
	ackErr        error
	nackErr       error
	rejectErr     error
}

func (f *fakeDeliveryActions) Ack(multiple bool) error {
	f.ackCalls++
	f.ackMultiple = multiple
	return f.ackErr
}

func (f *fakeDeliveryActions) Nack(multiple, requeue bool) error {
	f.nackCalls++
	f.nackMultiple = multiple
	f.nackRequeue = requeue
	return f.nackErr
}

func (f *fakeDeliveryActions) Reject(requeue bool) error {
	f.rejectCalls++
	f.rejectRequeue = requeue
	return f.rejectErr
}

func TestSettleDeliveryAcknowledgesSuccessfulHandler(t *testing.T) {
	delivery := &fakeDeliveryActions{}
	if err := settleDelivery(delivery, nil); err != nil {
		t.Fatalf("settleDelivery returned an error: %v", err)
	}
	if delivery.ackCalls != 1 || delivery.ackMultiple {
		t.Fatalf("expected one non-multiple ACK, got calls=%d multiple=%t", delivery.ackCalls, delivery.ackMultiple)
	}
	if delivery.nackCalls != 0 {
		t.Fatalf("unexpected NACK calls: %d", delivery.nackCalls)
	}
}

func TestSettleDeliveryRequeuesFailedHandler(t *testing.T) {
	delivery := &fakeDeliveryActions{}
	handlerErr := errors.New("database unavailable")
	if err := settleDelivery(delivery, handlerErr); err != nil {
		t.Fatalf("settleDelivery returned an error: %v", err)
	}
	if delivery.nackCalls != 1 || delivery.nackMultiple || !delivery.nackRequeue {
		t.Fatalf("expected one non-multiple requeue NACK, got calls=%d multiple=%t requeue=%t", delivery.nackCalls, delivery.nackMultiple, delivery.nackRequeue)
	}
	if delivery.ackCalls != 0 {
		t.Fatalf("unexpected ACK calls: %d", delivery.ackCalls)
	}
}

func TestSettleDeliveryRejectsPermanentHandlerFailure(t *testing.T) {
	delivery := &fakeDeliveryActions{}
	handlerErr := &PermanentDeliveryError{Err: errors.New("malformed payload")}
	if err := settleDelivery(delivery, handlerErr); err != nil {
		t.Fatalf("settleDelivery returned an error: %v", err)
	}
	if delivery.rejectCalls != 1 || delivery.rejectRequeue {
		t.Fatalf("expected one non-requeue reject, got calls=%d requeue=%t", delivery.rejectCalls, delivery.rejectRequeue)
	}
	if delivery.ackCalls != 0 || delivery.nackCalls != 0 {
		t.Fatalf("unexpected ACK/NACK calls: ack=%d nack=%d", delivery.ackCalls, delivery.nackCalls)
	}
}

func TestSettleDeliveryPropagatesSettlementError(t *testing.T) {
	ackErr := errors.New("ack failed")
	delivery := &fakeDeliveryActions{ackErr: ackErr}
	if err := settleDelivery(delivery, nil); !errors.Is(err, ackErr) {
		t.Fatalf("expected ACK error %v, got %v", ackErr, err)
	}

	nackErr := errors.New("nack failed")
	delivery = &fakeDeliveryActions{nackErr: nackErr}
	if err := settleDelivery(delivery, errors.New("handler failed")); !errors.Is(err, nackErr) {
		t.Fatalf("expected NACK error %v, got %v", nackErr, err)
	}

	rejectErr := errors.New("reject failed")
	delivery = &fakeDeliveryActions{rejectErr: rejectErr}
	if err := settleDelivery(delivery, &PermanentDeliveryError{Err: errors.New("bad payload")}); !errors.Is(err, rejectErr) {
		t.Fatalf("expected reject error %v, got %v", rejectErr, err)
	}
}

func TestRabbitMQPersistenceSettings(t *testing.T) {
	if !durableQueue {
		t.Fatal("async persistence requires a durable queue")
	}
	if persistentMessage != amqp.Persistent {
		t.Fatalf("expected persistent delivery mode, got %d", persistentMessage)
	}
}
