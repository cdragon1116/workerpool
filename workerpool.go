// Package workerpool ...
package workerpool

import (
	"context"
	"errors"
	"log"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

var (
	// ErrPoolClosed ...
	ErrPoolClosed = errors.New("pool closed")
	// ErrContextInterrupt ...
	ErrContextInterrupt = errors.New("context interrupt")
)

// WorkerPool ...
type WorkerPool struct {
	// Configurable settings
	MaxWorkers   int32
	PanicHandler func(interface{})

	// atomic counter
	activeWorkers int32

	// Private properties
	mutex     sync.Mutex
	closed    int32
	workersWg *sync.WaitGroup
}

// Option represents an option that can be passed when instantiating a worker pool to customize it
type Option func(*WorkerPool)

// defaultPanicHandler is the default panic handler
func defaultPanicHandler(panic interface{}) {
	log.Printf("Worker exits from a panic: %v\nStack trace: %s\n", panic, string(debug.Stack()))
}

// PanicHandler allows to change the panic handler function of a worker pool
func PanicHandler(panicHandler func(interface{})) Option {
	return func(pool *WorkerPool) {
		pool.PanicHandler = panicHandler
	}
}

// NewWorkerPool ...
// maxWorkers defines the maximum workers that can be execute tasks concurrently.
func NewWorkerPool(maxWorkers int32, options ...Option) *WorkerPool {
	if maxWorkers <= 0 {
		maxWorkers = int32(runtime.NumCPU() * 4)
	}

	pool := &WorkerPool{
		MaxWorkers:   maxWorkers,
		PanicHandler: defaultPanicHandler,
		closed:       0,
		workersWg:    new(sync.WaitGroup),
	}

	for _, opt := range options {
		opt(pool)
	}

	return pool
}

// AddTask sends a new goroutine to execute a task asynchronously.
// It will block when activeWorkers reached maximum config size.
// It takes context to interrupt the waiting.
func (w *WorkerPool) AddTask(ctx context.Context, fn func()) error {
	done := make(chan error)

	var stop int32 = 0

	go func() {
		var submitted bool

		for !submitted {
			if w.Closed() {
				done <- ErrPoolClosed
				return
			}

			if atomic.LoadInt32(&stop) == 1 {
				done <- ErrContextInterrupt
				return
			}

			submitted = w.incrementWorkerCount()

			if submitted {
				go w.worker(fn)
				close(done)
				break
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			atomic.StoreInt32(&stop, 1)
		case err := <-done:
			return err
		}
	}
}

func (w *WorkerPool) worker(task func()) {
	w.executeTask(task)
	w.decrementWorkerCount()
}

func (w *WorkerPool) executeTask(task func()) {
	defer func() {
		if panic := recover(); panic != nil {
			// Invoke panic handler
			w.PanicHandler(panic)
		}
	}()

	task()
}

func (w *WorkerPool) activeWorkerCount() int32 {
	return atomic.LoadInt32(&w.activeWorkers)
}

func (w *WorkerPool) incrementWorkerCount() bool {
	if w.activeWorkerCount() >= w.MaxWorkers {
		return false
	}

	w.mutex.Lock()
	defer w.mutex.Unlock()

	atomic.AddInt32(&w.activeWorkers, 1)
	w.workersWg.Add(1)

	return true
}

func (w *WorkerPool) decrementWorkerCount() {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	atomic.AddInt32(&w.activeWorkers, -1)

	w.workersWg.Done()
}

// Stop waits for all active workers finish then stop the workerpool.
// It takes context to interrupt the waiting.
// Once workerpool is closed, it can not be open again.
func (w *WorkerPool) Stop(ctx context.Context) error {
	done := make(chan struct{})

	go func() {
		defer close(done)
		w.close()
		w.WaitForTaskCompleted()
	}()

	for {
		select {
		case <-done:
			log.Println("workerPool: Stop: terminate successfully")
			return nil
		case <-ctx.Done():
			log.Printf("workerPool: Stop: %v", ctx.Err().Error())
			return ctx.Err()
		default:
		}
	}
}

// WaitForTaskCompleted waits for all active workers finish
func (w *WorkerPool) WaitForTaskCompleted() {
	w.workersWg.Wait()
}

// Close set worker pool state to close
func (w *WorkerPool) close() {
	atomic.StoreInt32(&w.closed, int32(1))
}

// Closed indicates workerpool closed
func (w *WorkerPool) Closed() bool {
	return atomic.LoadInt32(&w.closed) == 1
}
