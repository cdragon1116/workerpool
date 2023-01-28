# workerpool

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
![Coverage](https://img.shields.io/badge/Coverage-100.0%25-brightgreen)

Simple goroutine pool. Limits the concurrency of task execution.

## Installation

```
$ go get github.com/cdragon1116/workerpool
```

## Usage

- Create a new pool with max concurrency count.

```go
pool := workerpool.NewWorkerPool(10)
```

- Add Task

Sends a new worker to execute a task asynchronously.
It blocks when active workers reached maximum config size.
It returns error when interrupt by context or workerpool closed.

```go
err := pool.AddTask(context.Background(), func() {
    fmt.Println("hello world!")
})
```

- Wait for all task to complete

```go
pool.WaitForTaskCompleted(context.Background())
```

- Close pool

`Stop` waits for all active workers finish then stop the workerpool.
Once workerpool is closed, it can not be open again.
It takes a context to interrupt the waiting.

```go
pool.Stop(context.Background())
```

## Example

```go
ctx := context.Background()

// initialize a workerpool with max size = 10
pool := workerpool.NewWorkerPool(10)

// create a wait group to wait for task to enqueue
wg := new(sync.WaitGroup)

for i := 0; i < 15; i++ {
    wg.Add(1)

    go func() {
        defer wg.Done()

        pool.AddTask(context.Background(), func() {
            fmt.Println("hello world!")
            time.Sleep(1 * time.Second)
        })
    }()
}

// wait for all tasks enqueued
wg.Wait()

// wait for all tasks complete and exit
pool.Stop(ctx)
```

## Custom Configurations

- custom panic handler

```go
handFunc := func(interface{}) { fmt.Println("panic occurs!") }
pool := workerpool.NewWorkerPool(10, workerpool.PanicHandler(handFunc))
```
