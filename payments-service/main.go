package main

import (
	"context"
	"log"
	"os"
	"payments-service/handler"
	"payments-service/health"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/valyala/fasthttp"
)

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

	client := &fasthttp.Client{
		MaxConnsPerHost: 256,
		ReadTimeout:     700 * time.Millisecond,
		WriteTimeout:    700 * time.Millisecond,
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	requestHandler := func(ctx *fasthttp.RequestCtx) {
		path := string(ctx.Path())
		switch {
		case ctx.IsPost() && strings.HasPrefix(path, "/payments"):
			handler.PaymentHandler(defaultChecker, fallbackChecker)(ctx)
		case ctx.IsGet() && strings.HasPrefix(path, "/payments-summary"):
			handleProxy(ctx, client, "database:8888")
		case ctx.IsPost() && strings.HasPrefix(path, "/purge-payments"):
			handleProxy(ctx, client, "database:8888")
		default:
			ctx.Error("Not Found", fasthttp.StatusNotFound)
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8086"
	}
	log.Printf("Payments Service is running on port %s", port)
	if err := fasthttp.ListenAndServe(":"+port, requestHandler); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	log.Println("Payments Service stopped")
	log.Println("Exiting...")
}

func handleProxy(ctx *fasthttp.RequestCtx, client *fasthttp.Client, host string) {
	req := &ctx.Request
	resp := &ctx.Response
	req.SetHost(host)
	if err := client.Do(req, resp); err != nil {
		ctx.Error(err.Error(), fasthttp.StatusBadGateway)
	}
}
