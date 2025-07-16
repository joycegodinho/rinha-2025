package db

import "time"

type PaymentRecord struct {
	RequestedAt time.Time
	Amount      int64
	ServerType  string
}
