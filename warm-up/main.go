package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

type Payment struct {
	Amount        int    `json:"amount"`
	CorrelationID string `json:"correlation_id"`
}

func main() {
	fmt.Println("Starting full-chain warm-up...")

	lbURL := "http://load-balancer-backend:9999/payments"

	const warmupRequests = 50
	const concurrency = 10

	done := make(chan bool, concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			for j := 0; j < warmupRequests/concurrency; j++ {
				payload := Payment{
					Amount:        0,
					CorrelationID: uuid.NewString(),
				}
				body, _ := json.Marshal(payload)

				req, _ := http.NewRequest("POST", lbURL, bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					fmt.Println("Warm-up error:", err)
					continue
				}
				resp.Body.Close()
			}
			done <- true
		}()
	}

	for i := 0; i < concurrency; i++ {
		<-done
	}
	fmt.Println("Full-chain warm-up complete.")
}
