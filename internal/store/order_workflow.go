package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"sync"

	"github.com/infrai-examples/storemail-verification-service/internal/infrai"
)

var (
	ErrUnknownCustomer   = errors.New("customer not found")
	ErrUnverifiedEmail   = errors.New("email verification required before checkout")
	ErrUnknownOrder      = errors.New("order not found")
	ErrInvalidTransition = errors.New("order state transition is invalid")
)

type Mailer interface {
	SendEmail(context.Context, infrai.Email, string) (infrai.SendResult, error)
}

type Customer struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Verified bool   `json:"verified"`
	token    string
}

type Order struct {
	ID             string `json:"id"`
	CustomerID     string `json:"customer_id"`
	AmountCents    int    `json:"amount_cents"`
	State          string `json:"state"`
	ReceiptMessage string `json:"receipt_message_id,omitempty"`
	UpdateMessage  string `json:"update_message_id,omitempty"`
}

type Workflow struct {
	mu        sync.Mutex
	mailer    Mailer
	publicURL string
	newToken  func() (string, error)
	customers map[string]*Customer
	orders    map[string]*Order
}

func NewWorkflow(mailer Mailer, publicURL string) *Workflow {
	return &Workflow{
		mailer: mailer, publicURL: publicURL, newToken: randomToken,
		customers: make(map[string]*Customer), orders: make(map[string]*Order),
	}
}

func (w *Workflow) Signup(ctx context.Context, id, email string) (Customer, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if id == "" || email == "" {
		return Customer{}, errors.New("customer id and email are required")
	}
	token, err := w.newToken()
	if err != nil {
		return Customer{}, fmt.Errorf("create verification token: %w", err)
	}
	customer := &Customer{ID: id, Email: email, token: token}
	link := w.publicURL + "/verify?customer_id=" + id + "&token=" + token
	message := infrai.Email{
		To: email, Subject: "Verify your shop email",
		HTML: fmt.Sprintf("<p>Confirm this address before checkout:</p><p><a href=\"%s\">Verify email</a></p>", html.EscapeString(link)),
	}
	if _, err := w.mailer.SendEmail(ctx, message, "signup-verification:"+id); err != nil {
		return Customer{}, err
	}
	w.customers[id] = customer
	return *customer, nil
}

func (w *Workflow) Verify(customerID, token string) (Customer, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	customer, ok := w.customers[customerID]
	if !ok {
		return Customer{}, ErrUnknownCustomer
	}
	if token == "" || token != customer.token {
		return Customer{}, errors.New("verification token is invalid")
	}
	customer.Verified = true
	customer.token = ""
	return *customer, nil
}

func (w *Workflow) Checkout(ctx context.Context, orderID, customerID string, amountCents int) (Order, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	customer, ok := w.customers[customerID]
	if !ok {
		return Order{}, ErrUnknownCustomer
	}
	if !customer.Verified {
		return Order{}, ErrUnverifiedEmail
	}
	if orderID == "" || amountCents <= 0 {
		return Order{}, errors.New("order id and positive amount_cents are required")
	}
	order := &Order{ID: orderID, CustomerID: customerID, AmountCents: amountCents, State: "paid"}
	result, err := w.mailer.SendEmail(ctx, infrai.Email{
		To: customer.Email, Subject: "Receipt for order " + orderID,
		Body: fmt.Sprintf("Payment received for order %s: $%.2f.", orderID, float64(amountCents)/100),
	}, "order-receipt:"+orderID)
	if err != nil {
		return Order{}, err
	}
	order.ReceiptMessage = result.MessageID
	w.orders[orderID] = order
	return *order, nil
}

func (w *Workflow) Fulfill(ctx context.Context, orderID string) (Order, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	order, ok := w.orders[orderID]
	if !ok {
		return Order{}, ErrUnknownOrder
	}
	if order.State != "paid" {
		return Order{}, ErrInvalidTransition
	}
	customer := w.customers[order.CustomerID]
	result, err := w.mailer.SendEmail(ctx, infrai.Email{
		To: customer.Email, Subject: "Order " + orderID + " shipped",
		Body: "Your order has left fulfillment and is now shipped.",
	}, "order-shipped:"+orderID)
	if err != nil {
		return Order{}, err
	}
	order.State = "shipped"
	order.UpdateMessage = result.MessageID
	return *order, nil
}

func randomToken() (string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
