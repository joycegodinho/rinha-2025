package main

import (
	"fmt"
	"load-balancer-backend/config"
	"load-balancer-backend/health"
	"log"
	"net"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
)

type LoadBalancer struct {
	clients         []*fasthttp.HostClient
	roundRobinCount uint64
}

func (lb *LoadBalancer) nextIndex() int {
	return int(atomic.AddUint64(&lb.roundRobinCount, 1) % uint64(len(lb.clients)))
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	config.TuneGC()

	socketPaths := []string{
		"/sockets/payments-service-1.sock",
		"/sockets/payments-service-2.sock",
	}

	processorClient := &fasthttp.Client{
		MaxConnsPerHost:               10,
		ReadTimeout:                   2 * time.Second,
		WriteTimeout:                  2 * time.Second,
		MaxIdleConnDuration:           10 * time.Second,
		NoDefaultUserAgentHeader:      true,
		DisableHeaderNamesNormalizing: true,
		DisablePathNormalizing:        true,
		Dial: (&fasthttp.TCPDialer{
			Concurrency:      4096,
			DNSCacheDuration: time.Hour,
		}).Dial,
	}

	var clients []*fasthttp.HostClient
	dialer := &net.Dialer{
		Timeout:   200 * time.Millisecond,
		KeepAlive: 30 * time.Second,
	}
	for _, socketPath := range socketPaths {
		client := &fasthttp.HostClient{
			IsTLS: false,
			Dial: func(addr string) (net.Conn, error) {
				return dialer.Dial("unixpacket", socketPath)
			},
			ReadTimeout:         300 * time.Millisecond,
			WriteTimeout:        300 * time.Millisecond,
			MaxConns:            1024,
			MaxIdleConnDuration: 30 * time.Second,
			ReadBufferSize:      512,
			WriteBufferSize:     512,

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

	defaultChecker := &health.HealthManager{
		Processor:            "default",
		ProcessorEndpoint:    "http://payment-processor-default:8080/payments/service-health",
		HealthUpdateEndpoint: "/health-status-update", // Path on the Unix socket
		ProcessorClient:      processorClient,
		HealthUpdateClient:   clients,
	}
	go defaultChecker.CheckAndUpdateHealth()

	fallbackChecker := &health.HealthManager{
		Processor:            "fallback",
		ProcessorEndpoint:    "http://payment-processor-fallback:8080/payments/service-health",
		HealthUpdateEndpoint: "/health-status-update", // Path on the Unix socket
		ProcessorClient:      processorClient,
		HealthUpdateClient:   clients,
	}
	go fallbackChecker.CheckAndUpdateHealth()

	port := os.Getenv("PORT")
	if port == "" {
		port = "9999"
	}

	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Error creating listener: %v", err)
	}
	server := &fasthttp.Server{
		Handler:                       lb.Handler,
		ReadTimeout:                   700 * time.Millisecond,
		WriteTimeout:                  700 * time.Millisecond,
		ReadBufferSize:                1024,
		WriteBufferSize:               1024,
		DisableHeaderNamesNormalizing: true,
		IdleTimeout:                   30 * time.Second,
		NoDefaultDate:                 true,
		NoDefaultServerHeader:         true,
		NoDefaultContentType:          true,
		Concurrency:                   10000,
		DisableKeepalive:              false,
		DisablePreParseMultipartForm:  true,
		TCPKeepalive:                  true,
	}

	fmt.Println("Load balancer running on port:", port)

	if err := server.Serve(ln); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
