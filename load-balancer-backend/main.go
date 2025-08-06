package main

import (
	"encoding/json"
	"fmt"
	"load-balancer-backend/health"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
)

type LoadBalancer struct {
	clients         []*fasthttp.HostClient
	roundRobinCount uint64
}

type HealthInfo struct {
	DefaultFailing          bool `json:"defaultFailing"`
	DefaultMinResponseTime  int  `json:"defaultMinResponseTime"`
	FallbackFailing         bool `json:"fallbackFailing"`
	FallbackMinResponseTime int  `json:"fallbackMinResponseTime"`
}

var bodyPool = sync.Pool{
	New: func() any {
		return make([]byte, 0, 1024) // tuned size
	},
}

func (lb *LoadBalancer) Handler(ctx *fasthttp.RequestCtx) {
	if ctx.IsPost() && string(ctx.Path()) == "/payments" {
		lb.handlePayments(ctx)
		return
	}

	// Everything else: sync proxy
	next := lb.nextIndex()
	client := lb.clients[next]

	req := &ctx.Request
	resp := &ctx.Response

	req.SetHost("") // empty for unix
	if err := client.Do(req, resp); err != nil {
		ctx.Error(err.Error(), fasthttp.StatusBadGateway)
	}
}

func (lb *LoadBalancer) handlePayments(ctx *fasthttp.RequestCtx) {
	// Immediately accept the request
	ctx.SetStatusCode(fasthttp.StatusAccepted)

	// Copy body for async use
	raw := ctx.PostBody()
	buf := bodyPool.Get().([]byte)[:0]
	buf = append(buf, raw...) // copy

	go func(body []byte) {
		defer bodyPool.Put(body)
		defaultHealth := health.GetHealth("default")
		fallbackHealth := health.GetHealth("fallback")

		healthInfo := HealthInfo{
			DefaultFailing:          defaultHealth.Failing,
			DefaultMinResponseTime:  defaultHealth.MinResponseTime,
			FallbackFailing:         fallbackHealth.Failing,
			FallbackMinResponseTime: fallbackHealth.MinResponseTime,
		}

		var original map[string]any
		if err := json.Unmarshal(body, &original); err != nil {
			log.Printf("[LB] Async unmarshal error: %v", err)
			return
		}

		original["health_status"] = healthInfo

		newBody, err := json.Marshal(original)
		if err != nil {
			log.Printf("[LB] Async marshal error: %v", err)
			return
		}

		next := lb.nextIndex()
		client := lb.clients[next]

		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()
		defer fasthttp.ReleaseRequest(req)
		defer fasthttp.ReleaseResponse(resp)

		req.SetRequestURI("http://unix/payments")
		req.Header.SetMethod("POST")
		req.Header.SetContentType("application/json")
		req.SetHost("unix")
		req.SetBodyRaw(newBody)

		if err := client.Do(req, resp); err != nil {
			log.Printf("[LB] Async request error: %v", err)
		}
	}(buf)
}

func (lb *LoadBalancer) nextIndex() int {
	return int(atomic.AddUint64(&lb.roundRobinCount, 1) % uint64(len(lb.clients)))
}

func main() {
	// Start health checkers in the background
	defaultChecker := &health.HealthManager{
		Processor: "default",
		Endpoint:  "http://payment-processor-default:8080/payments/service-health",
	}
	go defaultChecker.CheckAndUpdateHealth()

	fallbackChecker := &health.HealthManager{
		Processor: "fallback",
		Endpoint:  "http://payment-processor-fallback:8080/payments/service-health",
	}
	go fallbackChecker.CheckAndUpdateHealth()

	// Paths to your Unix socket files (shared via Docker volume)
	socketPaths := []string{
		"/sockets/payments-service-1.sock",
		"/sockets/payments-service-2.sock",
	}

	var clients []*fasthttp.HostClient
	for _, socketPath := range socketPaths {
		// One HostClient per Unix socket
		client := &fasthttp.HostClient{
			IsTLS: false,
			Dial: func(addr string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
			ReadTimeout:  700 * time.Millisecond,
			WriteTimeout: 700 * time.Millisecond,
			MaxConns:     256,
			// MaxIdleConnDuration:           10 * time.Second,
			// MaxConnWaitTimeout:            100 * time.Millisecond,
			ReadBufferSize:                1024,
			WriteBufferSize:               1024,
			NoDefaultUserAgentHeader:      true,
			DisableHeaderNamesNormalizing: true,
			DisablePathNormalizing:        true,
		}
		clients = append(clients, client)
	}

	lb := &LoadBalancer{
		clients:         clients,
		roundRobinCount: 0,
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "9999"
	}

	fmt.Println("Load balancer running on port:", port)
	log.Fatal(fasthttp.ListenAndServe(":"+port, lb.Handler))
}
