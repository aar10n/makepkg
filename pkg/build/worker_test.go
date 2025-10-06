package build

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPool_BasicExecution(t *testing.T) {
	pool := NewWorkerPool(2)
	var counter int32

	for i := 0; i < 5; i++ {
		pool.Submit(func() {
			atomic.AddInt32(&counter, 1)
		})
	}

	pool.Wait()

	if counter != 5 {
		t.Errorf("Expected counter to be 5, got %d", counter)
	}
}

func TestWorkerPool_Concurrency(t *testing.T) {
	pool := NewWorkerPool(3)
	var running int32
	var maxConcurrent int32

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		pool.Submit(func() {
			defer wg.Done()
			current := atomic.AddInt32(&running, 1)
			if current > maxConcurrent {
				atomic.StoreInt32(&maxConcurrent, current)
			}
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&running, -1)
		})
	}

	pool.Wait()
	wg.Wait()

	if maxConcurrent > 3 {
		t.Errorf("Max concurrent workers was %d, should not exceed 3", maxConcurrent)
	}
	if maxConcurrent < 2 {
		t.Errorf("Max concurrent workers was %d, expected at least 2", maxConcurrent)
	}
}

func TestWorkerPool_ZeroWorkers(t *testing.T) {
	pool := NewWorkerPool(0)
	if pool.maxWorkers != 1 {
		t.Errorf("Expected pool with 0 workers to default to 1, got %d", pool.maxWorkers)
	}
}

func TestWorkerPool_NegativeWorkers(t *testing.T) {
	pool := NewWorkerPool(-5)
	if pool.maxWorkers != 1 {
		t.Errorf("Expected pool with negative workers to default to 1, got %d", pool.maxWorkers)
	}
}

func TestWorkerPool_SubmitWithStop_Normal(t *testing.T) {
	pool := NewWorkerPool(2)
	stopChan := make(chan struct{})
	var counter int32

	for i := 0; i < 3; i++ {
		pool.SubmitWithStop(func() {
			atomic.AddInt32(&counter, 1)
		}, stopChan)
	}

	pool.Wait()

	if counter != 3 {
		t.Errorf("Expected counter to be 3, got %d", counter)
	}
}

func TestWorkerPool_SubmitWithStop_Canceled(t *testing.T) {
	pool := NewWorkerPool(1)
	stopChan := make(chan struct{})
	var counter int32

	pool.SubmitWithStop(func() {
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&counter, 1)
	}, stopChan)

	close(stopChan)

	for i := 0; i < 5; i++ {
		pool.SubmitWithStop(func() {
			atomic.AddInt32(&counter, 1)
		}, stopChan)
	}

	pool.Wait()

	if counter > 1 {
		t.Errorf("Expected at most 1 task to complete after cancel, got %d", counter)
	}
}

func TestWorkerPool_SubmitWithStop_ImmediateCancel(t *testing.T) {
	pool := NewWorkerPool(2)
	stopChan := make(chan struct{})
	close(stopChan)

	var counter int32
	pool.SubmitWithStop(func() {
		atomic.AddInt32(&counter, 1)
	}, stopChan)

	pool.Wait()

	if counter != 0 {
		t.Errorf("Expected no tasks to run when stop channel already closed, got %d", counter)
	}
}

func TestWorkerPool_MultipleWaits(t *testing.T) {
	pool := NewWorkerPool(2)
	var counter int32

	pool.Submit(func() {
		atomic.AddInt32(&counter, 1)
	})

	pool.Wait()

	if counter != 1 {
		t.Errorf("Expected counter to be 1 after first wait, got %d", counter)
	}

	pool.Submit(func() {
		atomic.AddInt32(&counter, 1)
	})

	pool.Wait()

	if counter != 2 {
		t.Errorf("Expected counter to be 2 after second wait, got %d", counter)
	}
}
