package handler

import (
	"encoding/json"
	"log"
	"payments-service/health"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
)

type PaymentJob struct {
	CorrelationID string
	Amount        float64
	RequestedAt   time.Time
	Attempt       int
}

var fastClient = &fasthttp.Client{
	MaxIdleConnDuration: 30 * time.Second,
	MaxConnsPerHost:     200,
	ReadTimeout:         700 * time.Millisecond,
	WriteTimeout:        700 * time.Millisecond,
	//MaxConnWaitTimeout:        500 * time.Millisecond,
	//MaxIdemponentCallAttempts: 0,
}

var (
	retryQueue []PaymentJob
	retryMu    sync.Mutex
)

const MaxAttempts = 5

func AddToRetryQueue(job PaymentJob) {
	retryMu.Lock()
	defer retryMu.Unlock()
	retryQueue = append(retryQueue, job)
}

func PaymentHandler(defaultChecker, fallbackChecker *health.HealthManager) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		var req struct {
			CorrelationID string  `json:"correlationId"`
			Amount        float64 `json:"amount"`
		}

		json.Unmarshal(ctx.PostBody(), &req)

		job := PaymentJob{
			CorrelationID: req.CorrelationID,
			Amount:        req.Amount,
			RequestedAt:   time.Now().UTC(),
			Attempt:       0,
		}

		go ProcessPayment(job, defaultChecker, fallbackChecker)

		ctx.SetStatusCode(fasthttp.StatusAccepted)
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

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	req.SetRequestURI(endpoint)
	req.Header.SetMethod("POST")
	req.Header.SetContentType("application/json")
	req.SetBody(body)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	if err := fastClient.Do(req, resp); err != nil || resp.StatusCode() >= 500 {
		markProcessorAsFailing(processor, defaultChecker, fallbackChecker)
		job.Attempt++
		if job.Attempt < MaxAttempts {
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

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	req.SetRequestURI("http://database:8888/payments")
	req.Header.SetMethod("POST")
	req.Header.SetContentType("application/json")
	req.SetBody(body)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	if err := fastClient.Do(req, resp); err != nil {
		log.Printf("[DB] Error sending request: %v", err)
		return
	}

	if resp.StatusCode() >= 300 {
		log.Printf("[DB] Unexpected status: %d", resp.StatusCode())
	}
}

func StartRetryWorker(defaultChecker, fallbackChecker *health.HealthManager) {
	const workerCount = 30

	for i := 0; i < workerCount; i++ {
		go func(workerID int) {
			for {
				retryMu.Lock()
				if len(retryQueue) == 0 {
					retryMu.Unlock()
					time.Sleep(5 * time.Millisecond)
					continue
				}

				job := retryQueue[0]
				retryQueue = retryQueue[1:]
				retryMu.Unlock()

				ProcessPayment(job, defaultChecker, fallbackChecker)
			}
		}(i)
	}
}
