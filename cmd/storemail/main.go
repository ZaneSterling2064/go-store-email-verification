package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/infrai-examples/storemail-verification-service/internal/infrai"
	"github.com/infrai-examples/storemail-verification-service/internal/store"
)

type server struct{ workflow *store.Workflow }

func main() {
	client, err := infrai.NewClient(os.Getenv("INFRAI_API_KEY"))
	if err != nil {
		log.Fatal(err)
	}
	publicURL := os.Getenv("PUBLIC_URL")
	if publicURL == "" {
		publicURL = "http://127.0.0.1:8080"
	}
	s := server{workflow: store.NewWorkflow(client, publicURL)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /signup", s.signup)
	mux.HandleFunc("GET /verify", s.verify)
	mux.HandleFunc("POST /checkout", s.checkout)
	mux.HandleFunc("POST /fulfill", s.fulfill)
	log.Print("storemail listening on 127.0.0.1:8080")
	log.Fatal(http.ListenAndServe("127.0.0.1:8080", mux))
}

func (s server) signup(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CustomerID string `json:"customer_id"`
		Email      string `json:"email"`
	}
	if !decode(w, r, &input) {
		return
	}
	result, err := s.workflow.Signup(r.Context(), input.CustomerID, input.Email)
	writeResult(w, result, err)
}

func (s server) verify(w http.ResponseWriter, r *http.Request) {
	result, err := s.workflow.Verify(r.URL.Query().Get("customer_id"), r.URL.Query().Get("token"))
	writeResult(w, result, err)
}

func (s server) checkout(w http.ResponseWriter, r *http.Request) {
	var input struct {
		OrderID     string `json:"order_id"`
		CustomerID  string `json:"customer_id"`
		AmountCents int    `json:"amount_cents"`
	}
	if !decode(w, r, &input) {
		return
	}
	result, err := s.workflow.Checkout(r.Context(), input.OrderID, input.CustomerID, input.AmountCents)
	writeResult(w, result, err)
}

func (s server) fulfill(w http.ResponseWriter, r *http.Request) {
	var input struct {
		OrderID string `json:"order_id"`
	}
	if !decode(w, r, &input) {
		return
	}
	result, err := s.workflow.Fulfill(r.Context(), input.OrderID)
	writeResult(w, result, err)
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		http.Error(w, "invalid JSON request", http.StatusBadRequest)
		return false
	}
	return true
}

func writeResult(w http.ResponseWriter, result any, err error) {
	if err != nil {
		status := http.StatusBadRequest
		if !errors.Is(err, store.ErrUnknownCustomer) && !errors.Is(err, store.ErrUnknownOrder) &&
			!errors.Is(err, store.ErrUnverifiedEmail) && !errors.Is(err, store.ErrInvalidTransition) {
			status = http.StatusBadGateway
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Printf("encode response: %v", err)
	}
}
