package handler

import (
	"bytes"
	"log"
	"net"
	"os"
	"sync"
	"time"

	json "github.com/bytedance/sonic"

	"github.com/valyala/fasthttp"
)

type PaymentJob struct {
	CorrelationID string  `json:"correlationId"`
	Amount        float64 `json:"amount"`
	Attempt       int     `json:"attempt,omitempty"`
}
type ProcessorHealth struct {
	Failing         bool `json:"failing"`
	MinResponseTime int  `json:"minResponseTime"`
}

var (
	DefaultProcessorHealth  ProcessorHealth = ProcessorHealth{Failing: false, MinResponseTime: 0}
	FallbackProcessorHealth ProcessorHealth = ProcessorHealth{Failing: false, MinResponseTime: 0}
	healthMu                sync.RWMutex

	incomingQueue []*PaymentJob
	retryQueue    []*PaymentJob
	queueMu       sync.Mutex
)

type PaymentPayload struct {
	CorrelationID string    `json:"correlationId"`
	Amount        float64   `json:"amount"`
	RequestedAt   time.Time `json:"requestedAt"`
}

// select only default
// const (
// 	incomingWorkerCount = 8
// 	retryWorkerCount    = 10
// 	retryDelay          = 10 * time.Millisecond
// 	idleSleep           = 5 * time.Millisecond
// )

// select what is on
const (
	incomingWorkerCount = 10 // Number of workers for new payment requests
	retryWorkerCount    = 8  // Lower to avoid flooding when under pressure
	retryDelay          = 10 * time.Millisecond
	idleSleep           = 5 * time.Millisecond
)

var fastClient = &fasthttp.Client{
	MaxConnsPerHost:               512,
	ReadTimeout:                   700 * time.Millisecond,
	WriteTimeout:                  700 * time.Millisecond,
	ReadBufferSize:                1024,
	WriteBufferSize:               1024,
	NoDefaultUserAgentHeader:      true,
	DisableHeaderNamesNormalizing: true,
	DisablePathNormalizing:        true,
}

var DBClient = &fasthttp.Client{
	MaxConnsPerHost:               512,
	ReadTimeout:                   700 * time.Millisecond,
	WriteTimeout:                  700 * time.Millisecond,
	ReadBufferSize:                1024,
	WriteBufferSize:               1024,
	NoDefaultUserAgentHeader:      true,
	DisableHeaderNamesNormalizing: true,
	DisablePathNormalizing:        true,
	Dial: func(addr string) (net.Conn, error) {
		return net.Dial("unix", os.Getenv("DB_SOCKET_PATH"))
	},
}

var PayloadPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}
var PayloadDBPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

const MaxAttempts = 5

func PaymentHandler() fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		var job *PaymentJob

		// start := time.Now()
		_ = json.ConfigDefault.Unmarshal(ctx.PostBody(), &job)
		// fmt.Printf("Time to unmarshal: %v\n", time.Since(start))

		job.Attempt = 0

		EnqueueIncoming(job)

		ctx.SetStatusCode(fasthttp.StatusAccepted)
	}
}

func ProcessPayment(job *PaymentJob) bool {
	processor := SelectProcessor()
	if processor == "" {
		AddToRetryQueue(job)
		return false
	}

	endpoint := "http://payment-processor-default:8080/payments"
	if processor == "fallback" {
		endpoint = "http://payment-processor-fallback:8080/payments"
	}

	payload := PaymentPayload{
		CorrelationID: job.CorrelationID,
		Amount:        job.Amount,
		RequestedAt:   time.Now().UTC(),
	}

	buf := PayloadPool.Get().(*bytes.Buffer)
	defer PayloadPool.Put(buf)
	buf.Reset()

	if err := json.ConfigDefault.NewEncoder(buf).Encode(payload); err != nil {
		log.Printf("[Payment] Error encoding payload: %v", err)
		return false
	}

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	req.SetRequestURI(endpoint)
	req.Header.SetMethod("POST")
	req.Header.SetContentType("application/json")
	req.SetBodyRaw(buf.Bytes())

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	err := fastClient.DoTimeout(req, resp, 8*time.Second)
	if err != nil || resp.StatusCode() >= 500 {
		markProcessorAsFailing(processor)
		job.Attempt++
		if job.Attempt > MaxAttempts {
			log.Printf("[Payment] Max attempts reached for job: %v", job)
			return false
		}
		AddToRetryQueue(job)
		return false
	}

	SaveToDB(buf.Bytes(), processor)
	return true
}

func SaveToDB(body []byte, processor string) {
	trimmed := bytes.TrimRight(body, " \t\r\n")

	if len(trimmed) == 0 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		log.Printf("[DB] Unexpected JSON format, cannot append serverType")
		return
	}

	buf := PayloadDBPool.Get().(*bytes.Buffer)
	defer PayloadDBPool.Put(buf)
	buf.Reset()

	// Write original JSON without trailing '}'
	buf.Write(trimmed[:len(trimmed)-1])

	// Append , "serverType":"processor"}
	buf.WriteString(`,"serverType":"`)
	buf.WriteString(processor)
	buf.WriteString(`"}`)

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	req.SetRequestURI("http://unix/payments")
	req.Header.SetMethod("POST")
	req.Header.SetContentType("application/json")
	req.SetBodyRaw(buf.Bytes())

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	if err := DBClient.Do(req, resp); err != nil {
		log.Printf("[DB] Error sending request: %v", err)
		return
	}

	if resp.StatusCode() >= 300 {
		log.Printf("[DB] Unexpected status: %d", resp.StatusCode())
	}
}

// Select only default if not failing
// func SelectProcessor() string {
// 	healthMu.RLock()
// 	defer healthMu.RUnlock()

// 	if DefaultProcessorHealth.Failing && FallbackProcessorHealth.Failing {
// 		return ""
// 	}
// 	if !DefaultProcessorHealth.Failing {
// 		return "default"
// 	}
// 	return ""
// }

// // Selects the processor based on health status
// // const (
// //
// //	incomingWorkerCount = 15 // Number of workers for new payment requests
// //	retryWorkerCount    = 5  // Lower to avoid flooding when under pressure
// //	retryDelay          = 15 * time.Millisecond
// //	idleSleep           = 5 * time.Millisecond
// //
// // )
func SelectProcessor() string {
	healthMu.RLock()
	defer healthMu.RUnlock()

	if DefaultProcessorHealth.Failing && FallbackProcessorHealth.Failing {
		return ""
	}
	if !DefaultProcessorHealth.Failing {
		return "default"
	}
	return "fallback"
}

func markProcessorAsFailing(proc string) {
	healthMu.Lock()
	defer healthMu.Unlock()

	if proc == "default" {
		DefaultProcessorHealth.Failing = true
		DefaultProcessorHealth.MinResponseTime = 9999
	} else {
		FallbackProcessorHealth.Failing = true
		FallbackProcessorHealth.MinResponseTime = 9999
	}
}

func EnqueueIncoming(job *PaymentJob) {
	queueMu.Lock()
	incomingQueue = append(incomingQueue, job)
	queueMu.Unlock()
}

func AddToRetryQueue(job *PaymentJob) {
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
