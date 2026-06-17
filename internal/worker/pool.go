// Package worker provides a bounded-concurrency job runner used by the scan
// engine to fan out across many hosts without exhausting resources.
package worker

import (
	"context"
	"sync"
)

// Job is a unit of work producing a value of type T.
type Job[T any] func(ctx context.Context) (T, error)

// Result pairs a job's output with its error.
type Result[T any] struct {
	Value T
	Err   error
}

// Run executes jobs with at most concurrency workers running at once. Results
// are returned in the same order as jobs. If ctx is cancelled, not-yet-started
// jobs record ctx.Err() instead of running.
func Run[T any](ctx context.Context, concurrency int, jobs []Job[T]) []Result[T] {
	if concurrency < 1 {
		concurrency = 1
	}
	results := make([]Result[T], len(jobs))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, job := range jobs {
		if ctx.Err() != nil {
			results[i] = Result[T]{Err: ctx.Err()}
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(i int, job Job[T]) {
			defer wg.Done()
			defer func() { <-sem }()
			v, err := job(ctx)
			results[i] = Result[T]{Value: v, Err: err}
		}(i, job)
	}

	wg.Wait()
	return results
}
