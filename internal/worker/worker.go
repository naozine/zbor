package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"zbor/internal/storage"
	"zbor/internal/storage/sqlc"
)

// JobHandler is a function that processes a job
type JobHandler func(ctx context.Context, job *sqlc.ProcessingJob) error

// Worker processes jobs from the queue
type Worker struct {
	jobRepo  *storage.JobRepository
	handlers map[string]JobHandler
	interval time.Duration
	stop     chan struct{}
	wg       sync.WaitGroup
	mu       sync.RWMutex
}

// NewWorker creates a new worker
func NewWorker(jobRepo *storage.JobRepository) *Worker {
	return &Worker{
		jobRepo:  jobRepo,
		handlers: make(map[string]JobHandler),
		interval: 1 * time.Second,
		stop:     make(chan struct{}),
	}
}

// RegisterHandler registers a handler for a job type
func (w *Worker) RegisterHandler(jobType string, handler JobHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers[jobType] = handler
}

// SetInterval sets the polling interval
func (w *Worker) SetInterval(interval time.Duration) {
	w.interval = interval
}

// Start begins processing jobs
func (w *Worker) Start(ctx context.Context) {
	w.wg.Add(1)
	go w.run(ctx)
	log.Println("Worker started")
}

// Stop gracefully stops the worker
func (w *Worker) Stop() {
	close(w.stop)
	w.wg.Wait()
	log.Println("Worker stopped")
}

func (w *Worker) run(ctx context.Context) {
	defer w.wg.Done()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stop:
			return
		case <-ticker.C:
			w.processNextJob(ctx)
		}
	}
}

func (w *Worker) processNextJob(ctx context.Context) {
	job, err := w.jobRepo.GetNextQueued(ctx)
	if err != nil {
		log.Printf("Error getting next job: %v", err)
		return
	}
	if job == nil {
		return // No jobs to process
	}

	w.mu.RLock()
	handler, ok := w.handlers[job.Type]
	w.mu.RUnlock()

	if !ok {
		log.Printf("No handler for job type: %s", job.Type)
		_ = w.jobRepo.Fail(ctx, job.ID, "no handler registered for job type: "+job.Type)
		return
	}

	// Start the job
	if err := w.jobRepo.Start(ctx, job.ID); err != nil {
		log.Printf("Error starting job %s: %v", job.ID, err)
		return
	}

	log.Printf("Processing job %s (type: %s)", job.ID, job.Type)

	// Execute the handler
	if err := handler(ctx, job); err != nil {
		log.Printf("Job %s failed: %v", job.ID, err)
		w.handleJobFailure(ctx, job, err)
		return
	}

	// Complete the job
	if err := w.jobRepo.Complete(ctx, job.ID); err != nil {
		log.Printf("Error completing job %s: %v", job.ID, err)
		return
	}

	log.Printf("Job %s completed", job.ID)
}

func (w *Worker) handleJobFailure(ctx context.Context, job *sqlc.ProcessingJob, jobErr error) {
	retryCount := int64(0)
	if job.RetryCount != nil {
		retryCount = *job.RetryCount
	}

	maxRetries := int64(3)

	if retryCount < maxRetries {
		// Retry the job
		if err := w.jobRepo.Retry(ctx, job.ID); err != nil {
			log.Printf("Error retrying job %s: %v", job.ID, err)
		} else {
			log.Printf("Job %s queued for retry (attempt %d/%d)", job.ID, retryCount+1, maxRetries)
		}
	} else {
		// Max retries exceeded, mark as failed
		if err := w.jobRepo.Fail(ctx, job.ID, jobErr.Error()); err != nil {
			log.Printf("Error failing job %s: %v", job.ID, err)
		}
	}
}

// SubmitJob creates a new job and adds it to the queue
func (w *Worker) SubmitJob(ctx context.Context, jobType, sourceID string, priority int) (*sqlc.ProcessingJob, error) {
	job := &sqlc.ProcessingJob{
		Type:     jobType,
		SourceID: &sourceID,
		Priority: ptr(int64(priority)),
	}

	if err := w.jobRepo.Create(ctx, job); err != nil {
		return nil, err
	}

	log.Printf("Job %s submitted (type: %s, priority: %d)", job.ID, jobType, priority)
	return job, nil
}

func ptr[T any](v T) *T {
	return &v
}
