package handler

import (
	"database/api"
	"database/db"
	"encoding/json"
	"math"
	"net/http"
	"time"
)

func PaymentHandler(memoryDB *db.PaymentDB, fileDB *db.FileDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request api.PaymentRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		requestedAt, err := time.Parse(time.RFC3339, request.RequestedAt)
		if err != nil {
			http.Error(w, "Invalid requestedAt format. Use RFC3339 format", http.StatusBadRequest)
			return
		}

		if request.ServerType != "default" && request.ServerType != "fallback" {
			http.Error(w, "Invalid server type. Must be 'default' or 'fallback'", http.StatusBadRequest)
			return
		}

		record := &db.PaymentRecord{
			RequestedAt: requestedAt,
			Amount:      int64(math.Round(request.Amount * 100)),
			ServerType:  request.ServerType,
		}

		memoryDB.AddRecord(record)
		go fileDB.SaveRecord(record)

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "record created",
			"time":   requestedAt.Format(time.RFC3339),
		})
	}
}
