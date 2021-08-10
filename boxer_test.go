package boxer

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/google/go-cmp/cmp"

	"github.com/thesoulless/boxer/v2/job"

	"github.com/thesoulless/boxer/v2/retry"

	"github.com/google/uuid"
)

var (
	result = make(chan int)
)

type point struct {
	x float64
	y float64
}

func PowerN(_ context.Context, args ...interface{}) error {
	p := args[0].(*point)
	result <- int(math.Pow(p.x, p.y))
	return nil
}

func Power3(_ context.Context, _ ...interface{}) error {
	return nil
}

func ExampleBoxer() {
	// usually you do not need to call boxer.Terminate, since boxer will catch OS signals
	// and call the Terminate itself

	queue := "default"
	b, err := New(true, "myservice", "mysubservice", queue)
	if err != nil {
		panic(err)
	}

	b.Register("PowerN", PowerN)
	b.Run()

	j := job.New("PowerN", queue, 1, 0, &point{x: 2, y: 3})
	err = b.Push(j)
	if err != nil {
		panic(err)
	}
	time.Sleep(1 * time.Second)

	r := <-result

	fmt.Println(r)
	// Output: 8

	b.Terminate(false)
}

func getHighBoxer() *boxer {
	chs := make(jobPayload, 1)
	onFlyCounts := make(map[string]*int32, 1)
	for _, queue := range []string{"high"} {
		chs[queue] = make(chan *job.Job)
		zero := int32(0)
		onFlyCounts[queue] = &zero
	}
	defaultChs := make(jobPayload, 1)
	onFlyCountsDefault := make(map[string]*int32, 1)
	for _, queue := range []string{"default"} {
		defaultChs[queue] = make(chan *job.Job)
		zero := int32(0)
		onFlyCountsDefault[queue] = &zero
	}

	return &boxer{
		Concurrency:    runtime.NumCPU(),
		queues:         []string{"high"},
		done:           make(chan interface{}),
		shutdownWaiter: &sync.WaitGroup{},
		jobHandlers:    map[string]handler{},
		jobs:           chs,
		onFlyCounts:    onFlyCounts,
	}
}

func getDefaultBoxer() *boxer {
	defaultChs := make(jobPayload, 1)
	onFlyCountsDefault := make(map[string]*int32, 1)
	for _, queue := range []string{"default"} {
		defaultChs[queue] = make(chan *job.Job)
		zero := int32(0)
		onFlyCountsDefault[queue] = &zero
	}

	return &boxer{
		Concurrency:    runtime.NumCPU(),
		queues:         []string{"default"},
		done:           make(chan interface{}),
		shutdownWaiter: &sync.WaitGroup{},
		jobHandlers:    map[string]handler{},
		jobs:           defaultChs,
		onFlyCounts:    onFlyCountsDefault,
	}
}

//nolint:funlen
func TestNewBoxer(t *testing.T) {
	type args struct {
		withMetrics bool
		namespace   string
		subsystem   string
		queues      []string
	}

	tests := []struct {
		name    string
		args    args
		want    Boxer
		wantErr bool
	}{
		{
			name: "custom queue name",
			args: args{
				withMetrics: true,
				namespace:   "test1",
				subsystem:   "sub1",
				queues:      []string{"high"},
			},
			want: getHighBoxer(),
		},
		{
			name: "namespace subsystem collision",
			args: args{
				withMetrics: true,
				namespace:   "test1",
				subsystem:   "sub1",
				queues:      []string{"low"},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "no queue name",
			args: args{
				withMetrics: true,
				namespace:   "test3",
				subsystem:   "sub2",
				queues:      []string{},
			},
			want: getDefaultBoxer(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.args.withMetrics, tt.args.namespace, tt.args.subsystem, tt.args.queues...)
			if (err != nil) != tt.wantErr {
				t.Errorf("Fetch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !cmp.Equal(got, tt.want,
				cmp.Comparer(func(x, y *boxer) bool {
					for _, v := range y.queues {
						_, ok1 := x.onFlyCounts[v]
						_, ok2 := y.onFlyCounts[v]
						if !ok1 || !ok2 {
							return false
						}

						/*_, ok1 = x.jobs[v]
						_, ok2 = y.jobs[v]
						if !ok1 || !ok2 {
							return false
						}*/
					}

					// if x.count != y.count || x.Concurrency != y.Concurrency {
					//	return false
					//}

					return true
				}),
			) {
				t.Errorf("NewBoxer() = %v, want %v", got, tt.want)
				return
			}
		})
	}
}

func TestNewJob(t *testing.T) {
	type args struct {
		jobType    string
		queue      string
		maxRetries int
		delay      time.Duration
		args       []interface{}
	}

	args1 := []interface{}{
		[]string{"10", "11", "12"},
		uuid.New().String(),
	}

	tests := []struct {
		name string
		args args
		want *job.Job
	}{
		{
			name: "no queue name",
			args: args{
				jobType:    "PosChange",
				maxRetries: 3,
				args: []interface{}{
					[]string{"10", "11", "12"},
					uuid.New().String(),
				},
			},
			want: &job.Job{
				Type:      "PosChange",
				Queue:     "default",
				Args:      args1,
				CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
				Retry:     retry.New(3),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := job.New(tt.args.jobType, tt.args.queue, tt.args.maxRetries, tt.args.delay, tt.args.args...); !reflect.DeepEqual(got.Type, tt.want.Type) {
				t.Errorf("NewJob() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_boxer_Count(t *testing.T) {
	type fields struct {
		Concurrency    int
		queues         []string
		done           chan interface{}
		shutdownWaiter *sync.WaitGroup
		jobHandlers    map[string]handler
		wtr            bytes.Buffer
		jobs           jobPayload
	}

	queues := []string{"default"}

	chs := make(map[string]chan *job.Job)
	onFlyCounts := make(map[string]*int32, len(queues))
	for _, queue := range queues {
		chs[queue] = make(chan *job.Job)
		zero := int32(0)
		onFlyCounts[queue] = &zero
	}

	tests := []struct {
		name   string
		fields fields
		want   int
	}{
		{
			name: "zero jobs",
			fields: fields{
				Concurrency:    2,
				queues:         []string{"default"},
				done:           make(chan interface{}),
				shutdownWaiter: &sync.WaitGroup{},
				jobHandlers:    make(map[string]handler),
				wtr:            bytes.Buffer{},
				jobs:           chs,
			},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &boxer{
				Concurrency:    tt.fields.Concurrency,
				queues:         tt.fields.queues,
				done:           tt.fields.done,
				shutdownWaiter: tt.fields.shutdownWaiter,
				jobHandlers:    tt.fields.jobHandlers,
				jobs:           tt.fields.jobs,
			}
			if got := b.Count(); got != tt.want {
				t.Errorf("Count() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_boxer_Fail(t *testing.T) {
	type fields struct {
		Concurrency    int
		queues         []string
		done           chan interface{}
		shutdownWaiter *sync.WaitGroup
		shtdnLock      *sync.Mutex
		jobHandlers    map[string]handler
		wtr            bytes.Buffer
		jobs           jobPayload
	}
	type args struct {
		job *job.Job
	}

	queues := []string{"default"}
	chs := make(map[string]chan *job.Job)
	onFlyCounts := make(map[string]*int32, len(queues))
	for _, queue := range queues {
		chs[queue] = make(chan *job.Job)
		zero := int32(0)
		onFlyCounts[queue] = &zero
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "",
			fields: fields{
				Concurrency:    2,
				queues:         []string{"default"},
				done:           make(chan interface{}),
				shutdownWaiter: &sync.WaitGroup{},
				shtdnLock:      &sync.Mutex{},
				jobHandlers:    make(map[string]handler),
				wtr:            bytes.Buffer{},
				jobs:           chs,
			},
			args: args{
				job: job.New("Power2", "default", 0, 3),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &boxer{
				Concurrency:    tt.fields.Concurrency,
				queues:         tt.fields.queues,
				done:           tt.fields.done,
				shutdownWaiter: tt.fields.shutdownWaiter,
				jobHandlers:    tt.fields.jobHandlers,
				jobs:           tt.fields.jobs,
			}
			if err := b.fail(tt.args.job); (err != nil) != tt.wantErr {
				t.Errorf("fail() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

//nolint:funlen
func Test_boxer_Fetch(t *testing.T) {
	type fields struct {
		Concurrency    int
		queues         []string
		done           chan interface{}
		shutdownWaiter *sync.WaitGroup
		shtdnLock      *sync.Mutex
		jobHandlers    map[string]handler
		wtr            bytes.Buffer
		jobs           jobPayload
	}
	type args struct {
		q []string
	}
	queues := []string{"default"}
	chs := make(map[string]chan *job.Job)
	onFlyCounts := make(map[string]*int32, len(queues))
	for _, queue := range queues {
		chs[queue] = make(chan *job.Job)
		zero := int32(0)
		onFlyCounts[queue] = &zero
	}

	j := job.New("PowerN", "default", 0, 0, &point{x: 1, y: 4})
	go func() {
		time.Sleep(1 * time.Second)
		chs["default"] <- j
	}()

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *job.Job
		wantErr bool
	}{
		{
			name: "fetch a job",
			fields: fields{
				Concurrency:    2,
				queues:         []string{"default"},
				done:           make(chan interface{}),
				shutdownWaiter: &sync.WaitGroup{},
				shtdnLock:      &sync.Mutex{},
				jobHandlers:    make(map[string]handler),
				wtr:            bytes.Buffer{},
				jobs:           chs,
			},
			args:    args{q: []string{"default"}},
			want:    j,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &boxer{
				Concurrency:    tt.fields.Concurrency,
				queues:         tt.fields.queues,
				done:           tt.fields.done,
				shutdownWaiter: tt.fields.shutdownWaiter,
				jobHandlers:    tt.fields.jobHandlers,
				jobs:           tt.fields.jobs,
			}
			got, err := b.Fetch(tt.args.q...)
			if (err != nil) != tt.wantErr {
				t.Errorf("Fetch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got == nil || !assert.Equal(t, tt.want.ID, got.ID) {
				t.Errorf("Fetch() got = %v, want %v", got, tt.want)
			}
		})
	}
}

//nolint:funlen
func Test_boxer_Push(t *testing.T) {
	type fields struct {
		Concurrency    int
		queues         []string
		done           chan interface{}
		shutdownWaiter *sync.WaitGroup
		shtdnLock      *sync.Mutex
		jobHandlers    map[string]handler
		wtr            bytes.Buffer
		jobs           jobPayload
	}
	type args struct {
		job *job.Job
	}

	queues := []string{"default"}
	chs := make(map[string]chan *job.Job)
	onFlyCounts := make(map[string]*int32, len(queues))
	for _, queue := range queues {
		chs[queue] = make(chan *job.Job)
		zero := int32(0)
		onFlyCounts[queue] = &zero
	}

	j := job.New("PowerN", "default", 0, 0, &point{x: 2, y: 3})

	go func() {
		time.Sleep(1 * time.Second)
		<-chs["default"]
	}()

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "push a job",
			fields: fields{
				Concurrency:    2,
				queues:         []string{"default"},
				done:           make(chan interface{}),
				shutdownWaiter: &sync.WaitGroup{},
				shtdnLock:      &sync.Mutex{},
				jobHandlers:    make(map[string]handler),
				wtr:            bytes.Buffer{},
				jobs:           chs,
			},
			args:    args{job: j},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &boxer{
				Concurrency:    tt.fields.Concurrency,
				queues:         tt.fields.queues,
				done:           tt.fields.done,
				shutdownWaiter: tt.fields.shutdownWaiter,
				jobHandlers:    tt.fields.jobHandlers,
				jobs:           tt.fields.jobs,
			}
			if err := b.Push(tt.args.job); (err != nil) != tt.wantErr {
				t.Errorf("Push() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_dispatch(t *testing.T) {
	type args struct {
		ctx    context.Context
		job    *job.Job
		runner handler
	}

	var pow2 handler = func(ctx context.Context, job *job.Job) error {
		return nil
	}

	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "dispatch a job",
			args: args{
				ctx:    context.Background(),
				job:    job.New("Power2", "default", 0, 3),
				runner: pow2,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := dispatch(tt.args.ctx, tt.args.job, tt.args.runner); (err != nil) != tt.wantErr {
				t.Errorf("dispatch() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_jobContext(t *testing.T) {
	type args struct {
		job *job.Job
	}

	j1 := job.New("Power2", "default", 0, 3)
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "",
			args: args{job: j1},
			want: j1.ID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jobContext(tt.args.job); !reflect.DeepEqual(got.Value(jobKey).(string), tt.want) {
				t.Errorf("jobContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_processOne(t *testing.T) {
	type args struct {
		b *boxer
	}
	bi, _ := New(false, "ns", "sub", "default")
	b := bi.(*boxer)
	bi.Register("Power3", Power3)
	bi.Run()

	j := job.New("Power3", "default", 1, 3)

	go func() {
		time.Sleep(1 * time.Second)
		_ = bi.Push(j)
	}()

	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "",
			args:    args{b: b},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := processOne(tt.args.b); (err != nil) != tt.wantErr {
				t.Errorf("processOne() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_boxer_Terminate(t *testing.T) {
	type fields struct {
		Concurrency    int
		queues         []string
		done           chan interface{}
		shutdownWaiter *sync.WaitGroup
		shtdnLock      *sync.Mutex
		jobHandlers    map[string]handler
		jobs           jobPayload
		count          int32
		onFlyCounts    map[string]*int32
		jobErrors      chan *job.Error
	}
	type args struct {
		die bool
	}

	defaultChs := make(jobPayload, 1)
	onFlyCountsDefault := make(map[string]*int32, 1)
	for _, queue := range []string{"default"} {
		defaultChs[queue] = make(chan *job.Job)
		zero := int32(0)
		onFlyCountsDefault[queue] = &zero
	}

	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "ok",
			fields: fields{
				Concurrency:    runtime.NumCPU(),
				queues:         []string{"high"},
				done:           make(chan interface{}),
				shutdownWaiter: &sync.WaitGroup{},
				shtdnLock:      &sync.Mutex{},
				jobHandlers:    make(map[string]handler),
				jobs:           defaultChs,
				count:          0,
				onFlyCounts:    make(map[string]*int32, 1),
				jobErrors:      make(chan *job.Error),
			},
			args: args{
				die: false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &boxer{
				Concurrency:    tt.fields.Concurrency,
				queues:         tt.fields.queues,
				done:           tt.fields.done,
				shutdownWaiter: tt.fields.shutdownWaiter,
				jobHandlers:    tt.fields.jobHandlers,
				jobs:           tt.fields.jobs,
				count:          tt.fields.count,
				onFlyCounts:    tt.fields.onFlyCounts,
				jobErrors:      tt.fields.jobErrors,
			}
			b.Run()
			b.Terminate(tt.args.die)
		})
	}
}
