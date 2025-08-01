package main

import (
	"database/db"
	"database/handler"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/valyala/fasthttp"
)

func main() {
	fileDB := db.NewFileDB("./payments.json1")
	defer fileDB.Close()

	database := db.NewDB()

	if records, err := fileDB.LoadRecords(); err == nil {
		for _, record := range records {
			database.AddRecord(record)
		}
	} else {
		log.Printf("Failed to load initial records: %v", err)
	}

	paymentHandler := handler.PaymentHandler(database, fileDB)
	summaryHandler := handler.SummaryHandler(database)
	purgePaymentsHandler := handler.PurgePaymentsHandler(database, fileDB)

	requestHandler := func(ctx *fasthttp.RequestCtx) {
		switch string(ctx.Path()) {
		case "/payments":
			paymentHandler(ctx)
		case "/payments-summary":
			summaryHandler(ctx)
		case "/purge-payments":
			purgePaymentsHandler(ctx)
		default:
			ctx.Error("Unsupported path", fasthttp.StatusNotFound)
		}
	}

	socketPath := os.Getenv("SOCKET_PATH")
	if socketPath == "" {
		log.Fatal("SOCKET_PATH environment variable not set")
	}
	_ = os.Remove(socketPath)

	server := &fasthttp.Server{Handler: requestHandler}

	log.Printf("Database server starting on socket %s", socketPath)
	if err := server.ListenAndServeUNIX(socketPath, 0666); err != nil {
		log.Fatalf("Server error: %v", err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down Server...")

	if err := server.Shutdown(); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped...")
}
