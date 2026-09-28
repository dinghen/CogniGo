package rabbitmq

import (
	"errors"
	"testing"

	"github.com/streadway/amqp"
)

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
