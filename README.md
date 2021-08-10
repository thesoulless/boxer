# Boxer
![Tests](https://github.com/thesoulless/boxer/workflows/Tests/badge.svg?branch=master)
![](https://img.shields.io/github/v/tag/thesoulless/boxer?label=&logo=github&sort=semver)
![](https://img.shields.io/badge/godoc-docs-blue?label=&logo=go)

*I will work harder!*

*You're always right!*

**Boxer is an asynchronous in-memory Task Queue.**

## Workflow
1. Create a new boxer: boxer.New()
2. Register a runner: b.Register("MyCalcsRunner", MyCalcsRunner)
3. Run the boxer: b.Run()
4. Create a new job: job.New("MyCalcsRunner", ...)
5. Push the job to the boxer: b.Push(j)

## Dummy example

```go
package main

import (
	"context"
	"encoding/gob"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/thesoulless/boxer/v2"
	"github.com/thesoulless/boxer/v2/job"
)

func Power2(_ context.Context, args ...interface{}) error {
	x := args[0].(float64)
	result := int(math.Pow(2, x))
	f, err := os.Open("result.xyz")
	if err != nil {
		panic(err)
	}
	e := gob.NewEncoder(f)
	_ = e.Encode(result)
	_ = f.Close()
	return nil
}

func main() {
	queue := "default"
	b, err := boxer.New(true, "myservice", "mysubservice", queue)
	if err != nil {
		panic(err)
	}

	b.Register("Power2", Power2)
	b.Run()

	j := job.New("Power2", queue, 1, 3)
	err = b.Push(j)
	if err != nil {
		panic(err)
	}

	time.Sleep(100 * time.Millisecond)

    f, err := os.Open("result.xyz")
    if err != nil {
    	panic(err)
    }
	d := gob.NewDecoder(f)
	var res int
	_ = d.Decode(&res)
	fmt.Println(res)
	// Output: 8
}
```

## Known issues
Multiple queues are not functional yet.