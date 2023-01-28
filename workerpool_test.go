package workerpool_test

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"workerpool"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerpool_NewWorkerPool(t *testing.T) {
	size := int32(5)
	pool := workerpool.NewWorkerPool(size)

	assert.NotNil(t, pool)
	assert.Equal(t, size, pool.MaxWorkers)
	assert.NotNil(t, pool.PanicHandler)
}

func TestWorkerpool_NewWorkerPool_withOptions(t *testing.T) {
	size := int32(5)
	h := func(interface{}) {}
	pool := workerpool.NewWorkerPool(size, workerpool.PanicHandler(h))

	assert.NotNil(t, pool)
	assert.Equal(t, size, pool.MaxWorkers)

	func1 := runtime.FuncForPC(reflect.ValueOf(h).Pointer()).Name()
	func2 := runtime.FuncForPC(reflect.ValueOf(pool.PanicHandler).Pointer()).Name()
	assert.Equal(t, func1, func2)
}

func TestWorkerpool_NewWorkerPool_invalidInput(t *testing.T) {
	defaultSize := int32(runtime.NumCPU() * 4)
	pool := workerpool.NewWorkerPool(-5)

	assert.NotNil(t, pool)
	assert.Equal(t, defaultSize, pool.MaxWorkers)
}

func TestWorkerpool_AddTask(t *testing.T) {
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
	ctx, cancel := context.WithCancel(context.Background())

	task := func() { time.Sleep(10 * time.Second) }

	pool := NewWorkerpool(t, 1)
	pool.AddTask(ctx, task)

	var err error

	done := make(chan struct{})

	go func() {
		defer close(done)
		err = pool.AddTask(ctx, task)
	}()

	cancel()
	<-done

	assert.Equal(t, ctx.Err(), err)
}

func TestWorkerpool_AddTask_addTaskAfterPoolClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool := NewWorkerpool(t, 5)

	for i := 0; i < 5; i++ {
		pool.AddTask(ctx, func() {
			time.Sleep(10 * time.Second)
		})
	}

	done := make(chan error)

	go func() {
		done <- pool.AddTask(ctx, func() {})
	}()

	go pool.Stop(ctx)

	submitted := <-done

	assert.Equal(t, submitted, workerpool.ErrPoolClosed)
}

func TestWorkerpool_AddTask_tasksExceedsPoolSize(t *testing.T) {
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

	pool := workerpool.NewWorkerPool(5)
	err := pool.AddTask(ctx, func() { panic("test panic") })
	pool.WaitForTaskCompleted()

	assert.Nil(t, nil, err)
}

func TestWorkerpool_executeTask_customPanicHandle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := false

	h := func(interface{}) { done = true }

	pool := workerpool.NewWorkerPool(5, workerpool.PanicHandler(h))
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
				atomic.AddInt32(&doneCount, 1)
			})
		}

		err := pool.Stop(ctx)

		assert.NoError(t, err)
		assert.Equal(t, doneCount, int32(5), fmt.Sprintf("should have increment count %d", 5))
	})
}

func NewWorkerpool(t *testing.T, size int32) *workerpool.WorkerPool {
	pool := workerpool.NewWorkerPool(size)
	require.NotNil(t, pool)
	return pool
}
