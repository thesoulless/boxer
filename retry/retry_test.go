package retry

import "fmt"

func ExampleNew() {
	type Job struct {
		Name string
		Retry
	}

	job1 := Job{
		Name:  "First Job",
		Retry: New(3),
	}

	job1.Failed()

	job2 := Job{
		Name:  "First Job",
		Retry: New(3),
	}

	job2.Failed()
	job2.Failed()
	job2.Failed()

	fmt.Println(job1.Retried())
	fmt.Println(job1.CanRetry())

	fmt.Println(job2.CanRetry())
	// Output:
	// 1
	// true
	// false
}
