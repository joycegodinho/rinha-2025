package db

import (
	"bufio"
	"encoding/json"
	"errors"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type FileDB struct {
	filePath  string
	writeChan chan *PaymentRecord
	quitChan  chan struct{}
	wg        *sync.WaitGroup
	closed    uint32
	dropped   uint64
	mu        sync.RWMutex
}

func NewFileDB(filePath string) *FileDB {
	fdb := &FileDB{
		filePath:  filePath,
		writeChan: make(chan *PaymentRecord, 1000), // Buffered channel for async writes
		quitChan:  make(chan struct{}),
		wg:        &sync.WaitGroup{},
	}
	fdb.wg.Add(1)
	go fdb.asyncWriter()
	return fdb
}

func (f *FileDB) asyncWriter() {
	defer f.wg.Done()

	file, err := os.OpenFile(f.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()

	for {
		select {
		case record := <-f.writeChan:
			data, _ := json.Marshal(record)
			file.Write(append(data, '\n'))

		case <-f.quitChan:
			for len(f.writeChan) > 0 {
				record := <-f.writeChan
				data, _ := json.Marshal(record)
				file.Write(append(data, '\n'))
			}
			return
		}
	}
}

func (f *FileDB) DroppedCount() uint64 {
	return atomic.LoadUint64(&f.dropped)
}

func (f *FileDB) SaveRecord(record *PaymentRecord) error {
	f.mu.RLock()
	defer f.mu.RLock()

	if atomic.LoadUint32(&f.closed) == 1 {
		return errors.New("FileDB: closed, ignore writes")
	}

	select {
	case f.writeChan <- record:
		return nil
	default:
		atomic.AddUint64(&f.dropped, 1)
		select {
		case f.writeChan <- record:
			return nil
		case <-time.After(100 * time.Millisecond):
			log.Printf("FileDB: write channel full, dropping record %v", record)
			return errors.New("FileDB: write channel full, record dropped")
		}
	}
}

func (f *FileDB) LoadRecords() ([]*PaymentRecord, error) {
	f.mu.RLock()
	defer f.mu.RLock()

	file, err := os.Open(f.filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	records := []*PaymentRecord{}
	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		var record PaymentRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			log.Printf("FileDB: error parsing line %d: %v", lineNum, err)
			continue
		}
		records = append(records, &record)
	}
	return records, scanner.Err()
}

func (f *FileDB) EraseAll() error {
	if atomic.LoadUint32(&f.closed) == 1 {
		return errors.New("FileDB is already closed")
	}

	// Close current instance (flush pending records and stop writer)
	f.closeInternal()

	// Truncate the file to remove all data
	if err := os.Truncate(f.filePath, 0); err != nil {
		return err
	}

	// Reset state and restart async writer
	atomic.StoreUint32(&f.closed, 0)
	atomic.StoreUint64(&f.dropped, 0) // Reset drop counter
	f.quitChan = make(chan struct{})
	f.wg = &sync.WaitGroup{}
	f.wg.Add(1)
	go f.asyncWriter()

	return nil
}

func (f *FileDB) closeInternal() {
	if atomic.LoadUint32(&f.closed) == 1 {
		return
	}
	atomic.StoreUint32(&f.closed, 1)
	close(f.quitChan)
	f.wg.Wait()
}

func (f *FileDB) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closeInternal()
}
