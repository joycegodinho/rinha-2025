package main

import (
	"context"
	"log"
	"net"
	"os"
	"payments-service/handler"
	"payments-service/health"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/valyala/fasthttp"
)

var fastClient *fasthttp.Client

func main() {
	rdb := redis.NewClient(&redis.Options{
		Addr: "redis:6379",
	})

	ctx := context.Background()

	defaultChecker := &health.HealthManager{
		Redis:     rdb,
		Processor: "default",
		Endpoint:  "http://payment-processor-default:8080/payments/service-health",
		Ctx:       ctx,
	}
	go defaultChecker.CheckAndUpdateHealth()

	fallbackChecker := &health.HealthManager{
		Redis:     rdb,
		Processor: "fallback",
		Endpoint:  "http://payment-processor-fallback:8080/payments/service-health",
		Ctx:       ctx,
	}
	go fallbackChecker.CheckAndUpdateHealth()

	go handler.StartRetryWorker(defaultChecker, fallbackChecker)

	dbSocket := os.Getenv("DB_SOCKET_PATH")
	if dbSocket == "" {
		log.Fatal("DB_SOCKET_PATH not set")
	}

	fastClient = &fasthttp.Client{
		Dial: func(addr string) (net.Conn, error) {
			return net.Dial("unix", dbSocket)
		},
		ReadTimeout:  700 * time.Millisecond,
		WriteTimeout: 700 * time.Millisecond,
	}

	requestHandler := func(ctx *fasthttp.RequestCtx) {
		path := string(ctx.Path())
		switch {
		case ctx.IsPost() && strings.HasPrefix(path, "/payments"):
			handler.PaymentHandler(defaultChecker, fallbackChecker)(ctx)
		case ctx.IsGet() && strings.HasPrefix(path, "/payments-summary"):
			handleProxy(ctx)
		case ctx.IsPost() && strings.HasPrefix(path, "/purge-payments"):
			handleProxy(ctx)
		default:
			ctx.Error("Not Found", fasthttp.StatusNotFound)
		}
	}

	socketPath := os.Getenv("SOCKET_PATH")
	if socketPath == "" {
		log.Fatal("SOCKET_PATH environment variable not set")
	}
	_ = os.Remove(socketPath)

	log.Printf("Payments Service running on socket %s", socketPath)
	if err := fasthttp.ListenAndServeUNIX(socketPath, 0666, requestHandler); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	log.Println("Payments Service stopped")
	log.Println("Exiting...")
}

func handleProxy(ctx *fasthttp.RequestCtx) {
	req := &ctx.Request
	resp := &ctx.Response

	path := string(ctx.Path())
	query := string(ctx.QueryArgs().QueryString())

	fullURI := "http://unix" + path
	if query != "" {
		fullURI += "?" + query
	}

	req.SetRequestURI(fullURI)
	req.SetHost("unix")

	if err := fastClient.Do(req, resp); err != nil {
		ctx.Error(err.Error(), fasthttp.StatusBadGateway)
	}
}
