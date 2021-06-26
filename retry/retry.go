// Package retry
// Package retry provides type and methods to track retries on a
// desired object
//
// See example section for usage
package retry

import (
	"sync"
)

type Retry struct {
	MaxRetry int
	retried  int
	//sync.RWMutex
	sync.Mutex
}

// New create and returns a new Retry with MaxRetry set to max
// for max < 0, it will consider it as 0
func New(max int) Retry {
	if max < 0 {
		max = 0
	}
	return Retry{
		MaxRetry: max,
		retried:  0,
	}
}

// CanRetry return whether it can attempt fot another retry or
// it reached to it's max retries
func (r *Retry) CanRetry() bool {
	//r.RLock()
	//defer r.RUnlock()
	r.Lock()
	defer r.Unlock()
	return r.retried < r.MaxRetry
}

// Failed increases retries by 1
func (r *Retry) Failed() {
	r.Lock()
	defer r.Unlock()
	r.retried++
}

// Retried returns number of retried times
func (r *Retry) Retried() int {
	//r.RLock()
	//defer r.RUnlock()
	r.Lock()
	defer r.Unlock()
	return r.retried
}
