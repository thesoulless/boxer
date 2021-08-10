package job

import (
	"fmt"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBacktrace(t *testing.T) {
	type args struct {
		size int
	}

	_, filename, _, _ := runtime.Caller(0)
	_, testingFile, _, _ := runtime.Caller(1)
	_, asmFile, _, _ := runtime.Caller(2)
	trace1 := fmt.Sprintf("in %s:37 github.com/thesoulless/boxer/v2/job.TestBacktrace.func1", filename)
	trace2 := fmt.Sprintf("in %s:1193 testing.tRunner", testingFile)
	trace3 := fmt.Sprintf("in %s:1371 runtime.goexit", asmFile)
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "this caller trace",
			args: args{size: 10},
			want: []string{trace1, trace2, trace3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Backtrace(tt.args.size); !reflect.DeepEqual(got[0], tt.want[0]) || !reflect.DeepEqual(got[2], tt.want[2]) {
				t.Errorf("Backtrace() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	type args struct {
		jobType    string
		queue      string
		maxRetries int
		delay      time.Duration
		args       []interface{}
	}

	var a1 []interface{}
	a1 = append(a1, 3)

	tests := []struct {
		name string
		args args
		want *Job
	}{
		{
			name: "new",
			args: args{
				jobType:    "SomeJob",
				queue:      "default",
				maxRetries: 3,
				delay:      time.Second * 3,
				args:       a1,
			},
			want: &Job{
				Queue: "default",
				Type:  "SomeJob",
				Args:  a1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := New(tt.args.jobType, tt.args.queue, tt.args.maxRetries, tt.args.delay, tt.args.args...)
			if !assert.Equal(t, tt.want.Queue, got.Queue) || !assert.Equal(t, tt.want.Type, got.Type, tt.want.Args, got.Args) {
				t.Errorf("New() = %v, want %v", got, tt.want)
			}
		})
	}
}
