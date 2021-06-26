// Package boxer
// provides a local job processing functionality.
//
// General workflow:
//
// 1. Create a new boxer: boxer.New()
//
// 2. Register a runner: b.Register("MyCalcsRunner", MyCalcsRunner)
//
// 3. Run the boxer: b.Run()
//
// 4. Create a new job: job.New("MyCalcsRunner", ...)
//
// 5. Push the job to the boxer: b.Push(j)
package boxer

import (
	"context"
	"errors"
	"fmt"
	"log"
	mathrand "math/rand"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/thesoulless/boxer/job"

	"github.com/thesoulless/boxer/metrics"

	"sync/atomic"
)

// internal keys for context value storage
type valueKey int

const (
	jobKey valueKey = iota
)

// Each job processor should be implemented as a Runner type function
type Runner func(ctx context.Context, args ...interface{}) error

type NoRunnerError struct {
	JobType string
}

func (s *NoRunnerError) Error() string {
	return fmt.Sprintf("No runner is registered for job type %s", s.JobType)
}

type handler func(ctx context.Context, job *job.Job) error

type Boxer interface {
	Register(string, Runner)
	Run()
	Push(*job.Job) error
	Count() int
	OnFlyCounts() map[string]int32
	Error() <-chan *job.Error
	Terminate(bool)

	// Done can be useful for later implementing triggers
	// Done()
}

//type jobPayload map[string]chan []byte
type jobPayload map[string]chan *job.Job

type boxer struct {
	Concurrency int

	queues []string

	done           chan interface{}
	shutdownWaiter *sync.WaitGroup
	jobHandlers    map[string]handler

	jobs        jobPayload
	count       int32
	onFlyCounts map[string]*int32
	jobErrors   chan *job.Error
}

func New(withMetrics bool, namespace, subsystem string, queues ...string) (Boxer, error) {
	if len(queues) < 1 {
		queues = []string{"default"}
	}

	chs := make(map[string]chan *job.Job)
	onFlyCounts := make(map[string]*int32, len(queues))
	for _, queue := range queues {
		chs[queue] = make(chan *job.Job)
		zero := int32(0)
		onFlyCounts[queue] = &zero
	}

	b := &boxer{
		Concurrency:    runtime.NumCPU(),
		queues:         queues,
		done:           make(chan interface{}),
		shutdownWaiter: &sync.WaitGroup{},
		jobHandlers:    map[string]handler{},
		jobs:           chs,
		count:          0,
		jobErrors:      make(chan *job.Error),
		onFlyCounts:    onFlyCounts,
	}

	if withMetrics {
		err := metrics.New(b, namespace, subsystem)
		if err != nil {
			return nil, err
		}
	}

	return b, nil
}

func (b *boxer) Register(name string, fn Runner) {
	b.jobHandlers[name] = func(ctx context.Context, job *job.Job) error {
		//log.Printf("h %v", job.Args[0])
		//log.Printf("h2 %#v", &job.Args)
		return fn(ctx, job.Args...)
	}
}

func (b *boxer) Count() int {
	v := atomic.LoadInt32(&b.count)
	return int(v)
}

func (b *boxer) OnFlyCounts() map[string]int32 {
	onFlyVal := make(map[string]int32, len(b.onFlyCounts))

	for q, v := range b.onFlyCounts {
		_v := atomic.LoadInt32(v)
		onFlyVal[q] = _v
	}
	return onFlyVal
}

func (b *boxer) Run() {
	b.shutdownWaiter.Add(b.Concurrency)
	for i := 0; i < b.Concurrency; i++ {
		go process(b)
	}

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		killSignal := <-c
		switch killSignal {
		case os.Interrupt, syscall.SIGTERM:
			b.Terminate(true)
		}
	}()
}

func (b *boxer) Error() <-chan *job.Error {
	return b.jobErrors
}

func (b *boxer) Terminate(die bool) {
	b.done <- true
	b.shutdownWaiter.Wait()
	if die {
		os.Exit(0)
	}
}

func process(b *boxer) {
	// pause between 0 and 0.5B nanoseconds (0 - 0.5 seconds)
	time.Sleep(time.Duration(mathrand.Int31() / 4)) //nolint:gosec

	for {
		// check for shutdown
		select {
		case <-b.done:
			for i := 0; i < b.Concurrency; i++ {
				b.shutdownWaiter.Done()
			}
			return
		default:
		}

		err := processOne(b)
		if err != nil {
			var _e0 *NoRunnerError
			if ok := errors.Is(err, _e0); !ok {
				time.Sleep(1 * time.Second)
			}
		}
	}
}

func processOne(b *boxer) error {
	var (
		j   *job.Job
		err error
	)

	j, err = b.Fetch(b.queues...)
	if err != nil {
		return err
	}
	if j == nil {
		return nil
	}

	runner := b.jobHandlers[j.Type]

	if runner == nil {
		return &NoRunnerError{JobType: j.Type}
	}

	// increase onFlyCount when we're going to dispatch the job
	_, ok := b.onFlyCounts[j.Queue]
	if ok {
		atomic.AddInt32(b.onFlyCounts[j.Queue], 1)
	}

	log.Printf("j1 %v", j.Args[0])
	log.Printf("j2 %v", j.Args)
	joberr := dispatch(jobContext(j), j, runner)

	// decrease the onFlyCount
	if _, ok = b.onFlyCounts[j.Queue]; ok {
		atomic.AddInt32(b.onFlyCounts[j.Queue], -1)
	}

	if joberr != nil {
		log.Printf("error running %s job %s: %v", j.Type, j.ID, joberr)
		var backtrace []string

		if j.Backtrace > 0 {
			backtrace = job.Backtrace(j.Backtrace)
		}

		hostname, _ := os.Hostname()
		b.jobErrors <- &job.Error{
			HostName:   hostname,
			RetryCount: j.Retried(),
			FailedAt:   time.Now().UTC(),
			Backtrace:  backtrace,
			Err:        joberr,
		}

		res := b.fail(j)
		time.Sleep(1 * time.Second)
		return res
	}

	// decrease only when a job is done
	atomic.AddInt32(&b.count, -1)
	return nil
}

// fail is called when a job dispatch result has error.
func (b *boxer) fail(job *job.Job) error {
	if !job.CanRetry() {
		return nil
	}
	job.Failed()

	//jobBytes, err := json.Marshal(job)
	//if err != nil {
	//	return err
	//}

	//var buf bytes.Buffer
	//enc := gob.NewEncoder(&buf)
	//err := enc.Encode(job)
	//if err != nil {
	//	return err
	//}

	// pushes the job back to queue
	//b.jobs[job.Queue] <- buf.Bytes()
	b.jobs[job.Queue] <- job
	return nil
}

func jobContext(job *job.Job) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, jobKey, job.ID)
	return ctx
}

func dispatch(ctx context.Context, job *job.Job, runner handler) error {
	return runner(ctx, job)
}

func (b *boxer) Push(job *job.Job) error {
	log.Printf("bm1 %v", job.Args)
	//jobBytes, err := json.Marshal(job)
	//var buf bytes.Buffer
	//enc := gob.NewEncoder(&buf)
	//err := enc.Encode(job)
	//if err != nil {
	//	log.Printf("err %v", err)
	//	return err
	//}

	b.jobs[job.Queue] <- job
	//b.jobs[job.Queue] <- buf.Bytes()
	atomic.AddInt32(&b.count, 1)
	return nil
}

func (b *boxer) Fetch(q ...string) (*job.Job, error) {
	if len(q) == 0 {
		return nil, fmt.Errorf("fetch must be called with one or more queue names")
	}

	// @TODO use weighted round-robin to select the queue
	ch, ok := b.jobs[q[0]]
	if !ok {
		return nil, fmt.Errorf("channel unreachable %s", q[0])
	}
	data := <-ch

	//if len(data) == 0 {
	//	return nil, nil
	//}

	//var j job.Job
	//err := json.Unmarshal(data, &j)
	//buf := bytes.Buffer{}
	//buf.Write(data)
	//dec := gob.NewDecoder(&buf)
	//err := dec.Decode(&j)
	//if err != nil {
	//	return nil, err
	//}
	log.Printf("fm1 %v", data.Args)
	return data, nil
}
