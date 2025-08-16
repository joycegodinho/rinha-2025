package db

import (
	"database/api"
	"math"
	"sort"
	"sync"
	"time"
)

type MillisecondBucket struct {
	StartTime  time.Time // Start of the millisecond (truncated)
	Records    []*PaymentRecord
	ServerSums map[string]struct {
		Count int
		Sum   int64
	}
}

type PaymentDB struct {
	mu        sync.RWMutex
	buckets   map[int64]*MillisecondBucket // Key: UnixNano of millisecond
	timeIndex []*MillisecondBucket         // Sorted slice of buckets
}

func NewDB() *PaymentDB {
	return &PaymentDB{
		buckets:   make(map[int64]*MillisecondBucket),
		timeIndex: make([]*MillisecondBucket, 0),
	}
}

func (db *PaymentDB) insertToTimeIndex(bucket *MillisecondBucket) {
	index := sort.Search(len(db.timeIndex), func(i int) bool {
		return !db.timeIndex[i].StartTime.Before(bucket.StartTime)
	})
	db.timeIndex = append(db.timeIndex, nil)
	copy(db.timeIndex[index+1:], db.timeIndex[index:])
	db.timeIndex[index] = bucket
}

func (db *PaymentDB) AddRecord(record *PaymentRecord) {
	db.mu.Lock()
	defer db.mu.Unlock()

	truncated := record.RequestedAt.Truncate(time.Millisecond)
	key := truncated.UnixNano()

	bucket, exists := db.buckets[key]
	if !exists {
		bucket = &MillisecondBucket{
			StartTime: truncated,
			Records:   make([]*PaymentRecord, 0),
			ServerSums: map[string]struct {
				Count int
				Sum   int64
			}{
				"default":  {0, 0},
				"fallback": {0, 0},
			},
		}
		db.buckets[key] = bucket
		db.insertToTimeIndex(bucket)
	}

	bucket.Records = append(bucket.Records, record)

	sum := bucket.ServerSums[record.ServerType]
	sum.Count++
	sum.Sum += record.Amount
	bucket.ServerSums[record.ServerType] = sum
}

func (db *PaymentDB) QuerySummary(from, to time.Time) map[string]*api.Summary {
	db.mu.RLock()
	defer db.mu.RUnlock()

	// Initialize temporary result storage
	tempResult := map[string]struct {
		requests int
		cents    int64
	}{
		"default":  {0, 0},
		"fallback": {0, 0},
	}

	// Find bucket range boundaries with millisecond precision
	fromMS := from.Truncate(time.Millisecond)
	toMS := to.Truncate(time.Millisecond)

	// Find starting index using binary search
	startIdx := sort.Search(len(db.timeIndex), func(i int) bool {
		return !db.timeIndex[i].StartTime.Before(fromMS)
	})
	// Process buckets in time range
	for i := startIdx; i < len(db.timeIndex); i++ {
		bucket := db.timeIndex[i]
		bucketEnd := bucket.StartTime.Add(time.Millisecond)

		// Stop if we've passed the end of our time range
		if !to.IsZero() && bucket.StartTime.After(toMS) {
			break
		}

		// Check if bucket is entirely within query range
		if (from.IsZero() || !bucket.StartTime.Before(from)) &&
			(to.IsZero() || bucketEnd.Before(to) || bucketEnd.Equal(to)) {
			// Use pre-aggregated data for full bucket
			for serverType, sums := range bucket.ServerSums {
				data := tempResult[serverType]
				data.requests += sums.Count
				data.cents += sums.Sum
				tempResult[serverType] = data
			}
		} else {
			// Partial bucket - check individual records
			for _, record := range bucket.Records {
				if !from.IsZero() && record.RequestedAt.Before(from) {
					continue
				}
				if !to.IsZero() && !record.RequestedAt.Before(to) {
					continue
				}
				data := tempResult[record.ServerType]
				data.requests++
				data.cents += record.Amount
				tempResult[record.ServerType] = data
			}
		}
	}

	result := map[string]*api.Summary{
		"default":  {TotalRequests: 0, TotalAmount: 0},
		"fallback": {TotalRequests: 0, TotalAmount: 0},
	}

	for serverType, data := range tempResult {
		dollars := float64(data.cents) / 100.0
		dollars = math.Round(dollars*100) / 100 // Round to nearest cent
		result[serverType] = &api.Summary{
			TotalRequests: data.requests,
			TotalAmount:   dollars,
		}
	}

	return result
}

func (db *PaymentDB) Clean() {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.buckets = make(map[int64]*MillisecondBucket)
	db.timeIndex = make([]*MillisecondBucket, 0)
}
