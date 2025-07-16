package main

import (
	"context"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"payments-service/handler"
	"payments-service/health"
	"strings"

	"github.com/go-redis/redis/v8"
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

	summaryProxy := newReversedProxy("http://database:8888/payments-summary")
	purgePaymentsProxy := newReversedProxy("http://database:8888/purge-payments")

	router := http.NewServeMux()
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/payments"):
			handler.PaymentHandler(defaultChecker, fallbackChecker).ServeHTTP(w, r)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/payments-summary"):
			summaryProxy.ServeHTTP(w, r)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/purge-payments"):
			purgePaymentsProxy.ServeHTTP(w, r)

		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8086"
	}
	log.Printf("Payments Service is running on port %s", port)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	log.Println("Payments Service stopped")
	log.Println("Exiting...")
	log.Println("Goodbye!")

}

func newReversedProxy(target string) *httputil.ReverseProxy {
	targetURL, err := url.Parse(target)
	if err != nil {
		log.Fatalf("Invalid summary service URL: %v", err)
	}

	return &httputil.ReverseProxy{
		Director: func(r *http.Request) {
			// Preserve the full path for summary requests
			r.URL.Scheme = targetURL.Scheme
			r.URL.Host = targetURL.Host
			r.Host = targetURL.Host

			r.Header.Set("X-Forwarded-Host", r.Host)
			r.Header.Set("X-API-Gateway", "go-proxy/1.0")
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("Summary proxy error: %v", err)
			w.WriteHeader(http.StatusBadGateway)
		},
	}
}
