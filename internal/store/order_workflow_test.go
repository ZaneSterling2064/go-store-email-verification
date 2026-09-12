package store

import (
	"context"
	"errors"
	"testing"

	"github.com/infrai-examples/storemail-verification-service/internal/infrai"
)

type sentEmail struct {
	email infrai.Email
	key   string
}

type recordingMailer struct{ sent []sentEmail }

func (m *recordingMailer) SendEmail(_ context.Context, email infrai.Email, key string) (infrai.SendResult, error) {
	m.sent = append(m.sent, sentEmail{email: email, key: key})
	return infrai.SendResult{MessageID: "msg-" + key}, nil
}

func TestCheckoutRequiresVerifiedEmail(t *testing.T) {
	tests := []struct {
		name       string
		verify     bool
		wantErr    error
		wantState  string
		wantEmails int
	}{
		{name: "unverified customer is blocked", wantErr: ErrUnverifiedEmail, wantEmails: 1},
		{name: "verified customer receives receipt", verify: true, wantState: "paid", wantEmails: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mailer := &recordingMailer{}
			workflow := NewWorkflow(mailer, "http://127.0.0.1:8080")
			workflow.newToken = func() (string, error) { return "fixed-token", nil }
			if _, err := workflow.Signup(context.Background(), "customer-7", "buyer@example.com"); err != nil {
				t.Fatal(err)
			}
			if tt.verify {
				if _, err := workflow.Verify("customer-7", "fixed-token"); err != nil {
					t.Fatal(err)
				}
			}
			order, err := workflow.Checkout(context.Background(), "order-42", "customer-7", 2599)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Checkout() error = %v, want %v", err, tt.wantErr)
			}
			if order.State != tt.wantState {
				t.Fatalf("Checkout() state = %q, want %q", order.State, tt.wantState)
			}
			if len(mailer.sent) != tt.wantEmails {
				t.Fatalf("sent %d emails, want %d", len(mailer.sent), tt.wantEmails)
			}
		})
	}
}

func TestFulfillmentSendsOrderUpdate(t *testing.T) {
	mailer := &recordingMailer{}
	workflow := NewWorkflow(mailer, "http://127.0.0.1:8080")
	workflow.newToken = func() (string, error) { return "fixed-token", nil }
	ctx := context.Background()
	_, _ = workflow.Signup(ctx, "customer-7", "buyer@example.com")
	_, _ = workflow.Verify("customer-7", "fixed-token")
	_, _ = workflow.Checkout(ctx, "order-42", "customer-7", 2599)

	order, err := workflow.Fulfill(ctx, "order-42")
	if err != nil {
		t.Fatal(err)
	}
	if order.State != "shipped" || mailer.sent[2].key != "order-shipped:order-42" {
		t.Fatalf("fulfillment result = %+v, mail = %+v", order, mailer.sent[2])
	}
}
