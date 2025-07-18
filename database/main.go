package main

import (
	"context"
	"database/db"
	"database/handler"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	fileDB := db.NewFileDB("payments.json1")
	defer fileDB.Close()

	database := db.NewDB()

	if records, err := fileDB.LoadRecords(); err == nil {
		for _, record := range records {
			database.AddRecord(record)
		}
	} else {
		log.Printf("Failed to load initial records: %v", err)
	}

	http.HandleFunc("/payments", handler.PaymentHandler(database, fileDB))
	http.HandleFunc("/payments-summary", handler.SummaryHandler(database))
	http.HandleFunc("/purge-payments", handler.PurgePaymentsHandler(fileDB))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8888"
	}

	server := &http.Server{Addr: ":" + port}
	go func() {
		log.Printf("Database server starting on port %s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down Server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}
	log.Println("Server stopped")
}
