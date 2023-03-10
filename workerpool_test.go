package workerpool

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestWorkerpool_NewWorkerPool(t *testing.T) {
	size := int32(5)
	pool := NewWorkerPool(size)

	assert.NotNil(t, pool)
	assert.Equal(t, size, pool.MaxWorkers)
	assert.NotNil(t, pool.PanicHandler)
}

func TestWorkerpool_NewWorkerPool_withOptions(t *testing.T) {
	size := int32(5)
	h := func(interface{}) {}
	pool := NewWorkerPool(size, PanicHandler(h))

	assert.NotNil(t, pool)
	assert.Equal(t, size, pool.MaxWorkers)

	func1 := runtime.FuncForPC(reflect.ValueOf(h).Pointer()).Name()
	func2 := runtime.FuncForPC(reflect.ValueOf(pool.PanicHandler).Pointer()).Name()
	assert.Equal(t, func1, func2)
}

func TestWorkerpool_NewWorkerPool_invalidInput(t *testing.T) {
	defaultSize := int32(runtime.NumCPU() * 4)
	pool := NewWorkerPool(-5)

	assert.NotNil(t, pool)
	assert.Equal(t, defaultSize, pool.MaxWorkers)
}

func TestWorkerpool_AddTask(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	count := 0
	task := func() { count++ }

	pool := NewWorkerpool(t, 5)
	err := pool.AddTask(ctx, task)
	pool.WaitForTaskCompleted()

	assert.Nil(t, nil, err)
	assert.Equal(t, count, 1, fmt.Sprintf("should have increment count %d", 1))
}

func TestWorkerpool_AddTask_cancelByContext(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())

	signal := make(chan struct{})
	task := func() { <-signal }

	var err error
	pool := NewWorkerpool(t, 1)

	// 1st AddTask
	pool.AddTask(ctx, task)

	done := make(chan struct{})

	// 2nd AddTask
	go func() {
		defer close(done)
		err = pool.AddTask(ctx, task)
	}()

	// cancel context to interrupt 2nd AddTask
	cancel()

	// wait for 2nd Task return
	<-done

	// signal 1st Task to complete
	close(signal)

	// stop the pool
	pool.Stop(context.Background())

	assert.Equal(t, ErrContextInterrupt, err)
}

func TestWorkerpool_AddTask_addTaskAfterPoolClose(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewWorkerpool(t, 1)

	signal := make(chan error)

	done := make(chan error)

	go func() {
		<-signal
		fmt.Println("signal")
		done <- pool.AddTask(ctx, func() {})
	}()

	pool.Stop(ctx)

	close(signal)

	submitted := <-done

	assert.Equal(t, submitted, ErrPoolClosed)
}

func TestWorkerpool_AddTask_tasksExceedsPoolSize(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var doneCount int32

	pool := NewWorkerpool(t, 5)

	wg := new(sync.WaitGroup)

	for i := 0; i < 10; i++ {
		wg.Add(1)

		pool.AddTask(ctx, func() {
			defer wg.Done()
			atomic.AddInt32(&doneCount, 1)
		})
	}

	wg.Wait()

	assert.Equal(t, doneCount, int32(10))
}

func TestWorkerpool_executeTask_panicHandle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewWorkerPool(5)
	err := pool.AddTask(ctx, func() { panic("test panic") })
	pool.WaitForTaskCompleted()

	assert.Nil(t, nil, err)
}

func TestWorkerpool_executeTask_customPanicHandle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := false

	h := func(interface{}) { done = true }

	pool := NewWorkerPool(5, PanicHandler(h))
	pool.AddTask(ctx, func() { panic("test panic") })
	pool.WaitForTaskCompleted()

	assert.Equal(t, done, true)
}

func TestWorkerpool_Stop(t *testing.T) {
	t.Run("context cancelation", func(t *testing.T) {
		forever := make(chan struct{})

		ctx, cancel := context.WithCancel(context.Background())

		pool := NewWorkerpool(t, 5)

		pool.AddTask(ctx, func() {
			<-forever
		})

		var err error

		done := make(chan struct{})

		go func() {
			defer close(done)
			err = pool.Stop(ctx)
		}()

		cancel()

		<-done

		assert.ErrorIs(t, err, ctx.Err())
	})

	t.Run("ensure tasks completed", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var doneCount int32

		pool := NewWorkerpool(t, 5)

		for i := 0; i < 5; i++ {
			pool.AddTask(ctx, func() {
				time.Sleep(1 * time.Second)
				atomic.AddInt32(&doneCount, 1)
			})
		}

		var err error
		err = pool.Stop(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, doneCount, int32(5), fmt.Sprintf("should have increment count %d", 5))
	})
}

func NewWorkerpool(t *testing.T, size int32) *WorkerPool {
	pool := NewWorkerPool(size)
	require.NotNil(t, pool)
	return pool
}
