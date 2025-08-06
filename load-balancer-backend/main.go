package main

import (
	"fmt"
	"log"
	"net"
	"os"
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

func (lb *LoadBalancer) Handler(ctx *fasthttp.RequestCtx) {
	next := lb.nextIndex()
	client := lb.clients[next]

	req := &ctx.Request
	resp := &ctx.Response

	// No need to set Host for Unix socket — but you can leave it empty for clarity
	req.SetHost("")

	if err := client.Do(req, resp); err != nil {
		ctx.Error(err.Error(), fasthttp.StatusBadGateway)
	}
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
			ReadTimeout:                   700 * time.Millisecond,
			WriteTimeout:                  700 * time.Millisecond,
			MaxConns:                      256,
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
