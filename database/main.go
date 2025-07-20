package main

import (
	"database/db"
	"database/handler"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"
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

	requestHandler := func(ctx *fasthttp.RequestCtx) {
		switch string(ctx.Path()) {
		case "/payments":
			fasthttpadaptor.NewFastHTTPHandlerFunc(handler.PaymentHandler(database, fileDB))(ctx)
		case "/payments-summary":
			fasthttpadaptor.NewFastHTTPHandlerFunc(handler.SummaryHandler(database))(ctx)
		case "/purge-payments":
			fasthttpadaptor.NewFastHTTPHandlerFunc(handler.PurgePaymentsHandler(fileDB))(ctx)
		default:
			ctx.Error("Unsupported path", fasthttp.StatusNotFound)
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8888"
	}

	server := &fasthttp.Server{Handler: requestHandler}

	go func() {
		log.Printf("Database server starting on port %s", port)
		if err := server.ListenAndServe(":" + port); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down Server...")

	if err := server.Shutdown(); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped...")
}
