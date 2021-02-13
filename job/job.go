package job

import (
	cryptorand "crypto/rand"
	"encoding/base64"
	"fmt"
	mathrand "math/rand"
	"runtime"
	"time"

	"github.com/thesoulless/boxer/retry"
)

type Job struct {
	// Schedule should be used in order to implement task scheduling
	// Schedule string
	ID    string
	Queue string
	Type  string
	Args  []interface{}
	retry.Retry

	// optional
	CreatedAt string

	Backtrace int
}

func New(jobType string, queue string, maxRetries int, args ...interface{}) *Job {
	if queue == "" {
		queue = "default"
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	return &Job{
		Type:      jobType,
		Queue:     queue,
		Args:      args,
		ID:        RandomID(),
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Retry:     retry.New(maxRetries),
	}
}

func RandomID() string {
	b := make([]byte, 12)
	_, err := cryptorand.Read(b)
	if err != nil {
		mathrand.Read(b) //nolint:gosec
	}

	return base64.RawURLEncoding.EncodeToString(b)
}

type Error struct {
	HostName string

	RetryCount int
	FailedAt   time.Time
	Backtrace  []string

	Err error
}

// Backtrace gathers a backtrace for the caller.
// Return a slice of up to N stack frames.
func Backtrace(size int) []string {
	pc := make([]uintptr, size)
	n := runtime.Callers(2, pc)
	if n == 0 {
		return []string{}
	}

	pc = pc[:n] // pass only valid pcs to runtime.CallersFrames
	frames := runtime.CallersFrames(pc)

	str := make([]string, size)
	count := 0

	// get frames.
	for i := 0; i < size; i++ {
		frame, more := frames.Next()
		str[i] = fmt.Sprintf("in %s:%d %s", frame.File, frame.Line, frame.Function)
		count++
		if !more {
			break
		}
	}

	return str[0:count]
}
