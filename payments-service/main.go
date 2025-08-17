package main

import (
	"log"
	"net"
	"os"
	"payments-service/config"
	"payments-service/handler"
	"runtime"
	"strings"
	"time"

	"github.com/valyala/fasthttp"
)

var fastClient *fasthttp.Client

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	config.TuneGC()

	go handler.StartWorkers()

	dbSocket := os.Getenv("DB_SOCKET_PATH")
	if dbSocket == "" {
		log.Fatal("DB_SOCKET_PATH not set")
	}

	fastClient = &fasthttp.Client{
		Dial: func(addr string) (net.Conn, error) {
			return net.Dial("unix", dbSocket)
		},
		MaxConnsPerHost:               256,
		ReadTimeout:                   700 * time.Millisecond,
		WriteTimeout:                  700 * time.Millisecond,
		ReadBufferSize:                1024,
		WriteBufferSize:               1024,
		NoDefaultUserAgentHeader:      true,
		DisableHeaderNamesNormalizing: true,
		DisablePathNormalizing:        true,
	}

	requestHandler := func(ctx *fasthttp.RequestCtx) {
		path := string(ctx.Path())
		switch {
		case ctx.IsPost() && strings.HasPrefix(path, "/payments"):
			handler.PaymentHandler()(ctx)
		case ctx.IsPost() && strings.HasPrefix(path, "/health-status-update"):
			handler.HealthUpdateHandler()(ctx)
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

	ln, err := net.Listen("unixpacket", socketPath)
	if err != nil {
		log.Fatalf("Error creating UNIX listener: %v", err)
	}

	// Ensure correct permissions for other processes to access
	if err := os.Chmod(socketPath, 0666); err != nil {
		log.Fatalf("Failed to set socket permissions: %v", err)
	}

	server := &fasthttp.Server{
		Handler:                       requestHandler,
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
	}

	log.Printf("Payments Service running on socket %s", socketPath)
	if err := server.Serve(ln); err != nil {
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
