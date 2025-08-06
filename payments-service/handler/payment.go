package handler

import (
	"encoding/json"
	"log"
	"net"
	"os"
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

type ProcessorHealth struct {
	Failing         bool `json:"failing"`
	MinResponseTime int  `json:"minResponseTime"`
	// LastChecked     time.Time `json:"lastChecked,omitempty"`
}

type HealthInfo struct {
	DefaultFailing          bool `json:"defaultFailing"`
	DefaultMinResponseTime  int  `json:"defaultMinResponseTime"`
	FallbackFailing         bool `json:"fallbackFailing"`
	FallbackMinResponseTime int  `json:"fallbackMinResponseTime"`
}

var (
	DefaultProcessorHealth  ProcessorHealth
	FallbackProcessorHealth ProcessorHealth
	healthMu                sync.RWMutex

	incomingQueue []PaymentJob
	retryQueue    []PaymentJob
	queueMu       sync.Mutex
)

// const (
// 	incomingWorkerCount = 15 // Number of workers for new payment requests
// 	retryWorkerCount    = 5  // Lower to avoid flooding when under pressure
// 	retryDelay          = 15 * time.Millisecond
// 	idleSleep           = 5 * time.Millisecond
// )

const (
	incomingWorkerCount = 15 // Number of workers for new payment requests
	retryWorkerCount    = 10 // Lower to avoid flooding when under pressure
	retryDelay          = 5 * time.Millisecond
	idleSleep           = 5 * time.Millisecond
)

var fastClient = &fasthttp.Client{
	MaxConnsPerHost:               256,
	ReadTimeout:                   700 * time.Millisecond,
	WriteTimeout:                  700 * time.Millisecond,
	ReadBufferSize:                1024,
	WriteBufferSize:               1024,
	NoDefaultUserAgentHeader:      true,
	DisableHeaderNamesNormalizing: true,
	DisablePathNormalizing:        true,
}

var dbClient = &fasthttp.Client{
	Dial: func(addr string) (net.Conn, error) {
		return net.Dial("unix", os.Getenv("DB_SOCKET_PATH"))
	},
}

const MaxAttempts = 5

// func AddToRetryQueue(job PaymentJob) {
// 	if job.Attempt >= MaxAttempts {
// 		return
// 	}
// 	go func(j PaymentJob) {
// 		time.Sleep(time.Millisecond * time.Duration(j.Attempt*10))
// 		select {
// 		case retryQueue <- j:
// 		default:
// 			log.Println("[RetryQueue] Full, dropping job")
// 		}
// 	}(job)
// }

func PaymentHandler() fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		var reqBody struct {
			CorrelationID string     `json:"correlationId"`
			Amount        float64    `json:"amount"`
			HealthStatus  HealthInfo `json:"health_status"`
		}

		_ = json.Unmarshal(ctx.PostBody(), &reqBody)

		// Update global health info
		healthMu.Lock()
		DefaultProcessorHealth = ProcessorHealth{
			Failing:         reqBody.HealthStatus.DefaultFailing,
			MinResponseTime: reqBody.HealthStatus.DefaultMinResponseTime,
			// LastChecked:     time.Now().UTC(),
		}
		FallbackProcessorHealth = ProcessorHealth{
			Failing:         reqBody.HealthStatus.FallbackFailing,
			MinResponseTime: reqBody.HealthStatus.FallbackMinResponseTime,
			// LastChecked:     time.Now().UTC(),
		}
		healthMu.Unlock()

		job := PaymentJob{
			CorrelationID: reqBody.CorrelationID,
			Amount:        reqBody.Amount,
			RequestedAt:   time.Now().UTC(),
			Attempt:       0,
		}

		EnqueueIncoming(job)

		ctx.SetStatusCode(fasthttp.StatusAccepted)
	}
}

// Selects the processor based on health status
// func SelectProcessor() string {
// 	healthMu.RLock()
// 	defer healthMu.RUnlock()

// 	if DefaultProcessorHealth.Failing && FallbackProcessorHealth.Failing {
// 		return ""
// 	}
// 	if !DefaultProcessorHealth.Failing {
// 		return "default"
// 	}
// 	return "fallback"
// }

// // Select only default if not failing
func SelectProcessor() string {
	healthMu.RLock()
	defer healthMu.RUnlock()

	if DefaultProcessorHealth.Failing && FallbackProcessorHealth.Failing {
		return "" // Both failing
	}
	if !DefaultProcessorHealth.Failing {
		return "default"
	}
	return "" // Fallback is not used in this logic if default is failing
}

// Select processor by time
// func SelectProcessor() string {
// 	healthMu.RLock()
// 	defer healthMu.RUnlock()

// 	if DefaultProcessorHealth.Failing && FallbackProcessorHealth.Failing {
// 		return ""
// 	}
// 	if !DefaultProcessorHealth.Failing && FallbackProcessorHealth.Failing {
// 		return "default"
// 	}
// 	if DefaultProcessorHealth.Failing && !FallbackProcessorHealth.Failing {
// 		return "fallback"
// 	}
// 	if DefaultProcessorHealth.MinResponseTime <= FallbackProcessorHealth.MinResponseTime {
// 		return "default"
// 	}
// 	return "fallback"
// }

// Select processor by time with graceful lag
// const (
// 	gracefulLag = 100
// )

// func SelectProcessor() string {
// 	healthMu.RLock()
// 	defer healthMu.RUnlock()

// 	if DefaultProcessorHealth.Failing && FallbackProcessorHealth.Failing {
// 		return ""
// 	}
// 	if !DefaultProcessorHealth.Failing && FallbackProcessorHealth.Failing {
// 		return "default"
// 	}
// 	if DefaultProcessorHealth.Failing && !FallbackProcessorHealth.Failing {
// 		return "fallback"
// 	}
// 	if DefaultProcessorHealth.MinResponseTime <= FallbackProcessorHealth.MinResponseTime+gracefulLag {
// 		return "default"
// 	}
// 	return "fallback"
// }

func markProcessorAsFailing(proc string) {
	healthMu.Lock()
	defer healthMu.Unlock()

	if proc == "default" {
		DefaultProcessorHealth.Failing = true
		DefaultProcessorHealth.MinResponseTime = 9999
		// DefaultProcessorHealth.LastChecked = time.Now().UTC()
	} else {
		FallbackProcessorHealth.Failing = true
		FallbackProcessorHealth.MinResponseTime = 9999
		// FallbackProcessorHealth.LastChecked = time.Now().UTC()
	}
}

func ProcessPayment(job PaymentJob) bool {
	processor := SelectProcessor()
	if processor == "" {
		job.Attempt++
		AddToRetryQueue(job)
		return false
	}

	endpoint := "http://payment-processor-default:8080/payments"
	if processor == "fallback" {
		endpoint = "http://payment-processor-fallback:8080/payments"
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
	req.SetBodyRaw(body)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	err := fastClient.DoTimeout(req, resp, 6*time.Second)
	if err != nil || resp.StatusCode() >= 500 {
		markProcessorAsFailing(processor)
		job.Attempt++
		AddToRetryQueue(job)
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
	req.SetRequestURI("http://unix/payments")
	req.Header.SetMethod("POST")
	req.Header.SetContentType("application/json")
	req.SetBodyRaw(body)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	if err := dbClient.Do(req, resp); err != nil {
		log.Printf("[DB] Error sending request: %v", err)
		return
	}

	if resp.StatusCode() >= 300 {
		log.Printf("[DB] Unexpected status: %d", resp.StatusCode())
	}
}

func EnqueueIncoming(job PaymentJob) {
	queueMu.Lock()
	incomingQueue = append(incomingQueue, job)
	queueMu.Unlock()
}

func AddToRetryQueue(job PaymentJob) {
	queueMu.Lock()
	retryQueue = append(retryQueue, job)
	queueMu.Unlock()
}

func StartWorkers() {
	for i := 0; i < incomingWorkerCount; i++ {
		go incomingWorker(i)
	}
	for i := 0; i < retryWorkerCount; i++ {
		go retryWorker(i)
	}
}

func incomingWorker(id int) {
	for {
		queueMu.Lock()
		if len(incomingQueue) == 0 {
			queueMu.Unlock()
			time.Sleep(idleSleep)
			continue
		}
		job := incomingQueue[0]
		incomingQueue = incomingQueue[1:]
		queueMu.Unlock()

		// start := time.Now()
		ProcessPayment(job)
		// log.Printf("[Worker %d] Incoming processed in %v", id, time.Since(start))
	}
}

func retryWorker(id int) {
	for {
		queueMu.Lock()
		if len(retryQueue) == 0 {
			queueMu.Unlock()
			time.Sleep(idleSleep)
			continue
		}
		job := retryQueue[0]
		retryQueue = retryQueue[1:]
		queueMu.Unlock()

		time.Sleep(retryDelay)

		// start := time.Now()
		ProcessPayment(job)
		// log.Printf("[RetryWorker %d] Retry processed in %v", id, time.Since(start))
	}
}
