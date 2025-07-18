package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"log"

	// "log"
	"net/http"
	"payments-service/health"
	"sync"
	"time"
)

type PaymentJob struct {
	CorrelationID string
	Amount        float64
	RequestedAt   time.Time
	Attempt       int
}

var fastClient = &http.Client{
	Timeout: 700 * time.Millisecond, // faster failover
	Transport: &http.Transport{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		MaxConnsPerHost:       100,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   300 * time.Millisecond,
		ExpectContinueTimeout: 0,
	},
}

var (
	retryQueue []PaymentJob
	retryMu    sync.Mutex
)

const MaxAttempts = 3

func AddToRetryQueue(job PaymentJob) {
	retryMu.Lock()
	defer retryMu.Unlock()
	retryQueue = append(retryQueue, job)
	// log.Printf("[RetryQueue] Added job %s (attempt %d). Queue length: %d", job.CorrelationID, job.Attempt, len(retryQueue))
}

func PaymentHandler(defaultChecker, fallbackChecker *health.HealthManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CorrelationID string  `json:"correlationId"`
			Amount        float64 `json:"amount"`
		}

		json.NewDecoder(r.Body).Decode(&req)

		job := PaymentJob{
			CorrelationID: req.CorrelationID,
			Amount:        req.Amount,
			RequestedAt:   time.Now().UTC(),
			Attempt:       0,
		}

		go ProcessPayment(job, defaultChecker, fallbackChecker)

		w.WriteHeader(http.StatusAccepted)
	}
}

func SelectProcessor(defaultHealth, fallbackHealth *health.ProcessorHealth) string {
	if defaultHealth.Failing && fallbackHealth.Failing {
		return ""
	}
	if !defaultHealth.Failing && fallbackHealth.Failing {
		return "default"
	}
	if defaultHealth.Failing && !fallbackHealth.Failing {
		return "fallback"
	}

	if defaultHealth.MinResponseTime <= fallbackHealth.MinResponseTime {
		return "default"
	}
	return "fallback"
}

func markProcessorAsFailing(proc string, d *health.HealthManager, f *health.HealthManager) {
	if proc == "default" {
		d.SaveHealthToRedis(true, 9999)
	} else {
		f.SaveHealthToRedis(true, 9999)
	}
}

func ProcessPayment(job PaymentJob, defaultChecker, fallbackChecker *health.HealthManager) bool {
	defaultHealth, _ := defaultChecker.GetHealth()
	fallbackHealth, _ := fallbackChecker.GetHealth()

	processor := SelectProcessor(defaultHealth, fallbackHealth)
	if processor == "" {
		AddToRetryQueue(job)
		return false
	}

	endpoint := "http://payment-processor-fallback:8080/payments"

	if processor == "default" {
		endpoint = "http://payment-processor-default:8080/payments"
	}
	payload := map[string]any{
		"correlationId": job.CorrelationID,
		"amount":        job.Amount,
		"requestedAt":   job.RequestedAt,
	}

	body, _ := json.Marshal(payload)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := fastClient.Do(req)
	if err != nil || resp.StatusCode >= 500 {
		markProcessorAsFailing(processor, defaultChecker, fallbackChecker)
		// Retry
		job.Attempt++
		if job.Attempt < 5 {
			AddToRetryQueue(job)
		}
		return false
	}

	SaveToDB(job, processor)
	return true

}

func SaveToDB(job PaymentJob, processor string) {
	payload := map[string]any{
		"amount":      job.Amount,
		"serverType":  processor,
		"requestedAt": job.RequestedAt,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[DB] Error marshaling payload: %v", err)
		return
	}

	req, err := http.NewRequest("POST", "http://database:8888/payments", bytes.NewBuffer(body))
	if err != nil {
		log.Printf("[DB] Error creating request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := fastClient.Do(req)
	if err != nil {
		log.Printf("[DB] Error sending request: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		log.Printf("[DB] Unexpected status: %d", resp.StatusCode)
	}
}

func StartRetryWorker(defaultChecker, fallbackChecker *health.HealthManager) {
	const workerCount = 20 // You can tune this depending on CPU/mem usage

	for i := 0; i < workerCount; i++ {
		go func(workerID int) {
			for {
				// log.Printf("[RetryWorker] Running")
				retryMu.Lock()
				if len(retryQueue) == 0 {
					retryMu.Unlock()
					time.Sleep(5 * time.Millisecond)
					continue
				}

				job := retryQueue[0]
				retryQueue = retryQueue[1:]
				retryMu.Unlock()

				// log.Printf("[RetryWorker-%d] Retrying job %s (attempt %d)", workerID, job.CorrelationID, job.Attempt)
				ProcessPayment(job, defaultChecker, fallbackChecker)
				// success := ProcessPayment(job, defaultChecker, fallbackChecker)
				// if !success && job.Attempt < MaxAttempts {
				// 	// job.Attempt++
				// 	// AddToRetryQueue(job)
				// }

			}
		}(i)
	}
}
