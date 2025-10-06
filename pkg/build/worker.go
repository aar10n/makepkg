package build

import (
	"sync"
)

// WorkerPool manages concurrent execution of tasks.
type WorkerPool struct {
	maxWorkers int
	sem        chan struct{}
	wg         sync.WaitGroup
}

// NewWorkerPool creates a new worker pool with the specified number of workers.
func NewWorkerPool(maxWorkers int) *WorkerPool {
	if maxWorkers <= 0 {
		maxWorkers = 1
	}
	return &WorkerPool{
		maxWorkers: maxWorkers,
		sem:        make(chan struct{}, maxWorkers),
	}
}

// Submit submits a task to the worker pool.
func (p *WorkerPool) Submit(task func()) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.sem <- struct{}{}
		defer func() { <-p.sem }()
		task()
	}()
}

// SubmitWithStop submits a task that can be canceled via a stop channel.
func (p *WorkerPool) SubmitWithStop(task func(), stopChan <-chan struct{}) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		select {
		case p.sem <- struct{}{}:
			defer func() { <-p.sem }()
		case <-stopChan:
			return
		}

		select {
		case <-stopChan:
			return
		default:
			task()
		}
	}()
}

// Wait waits for all submitted tasks to complete.
func (p *WorkerPool) Wait() {
	p.wg.Wait()
}
