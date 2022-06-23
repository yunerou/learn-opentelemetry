// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"go.opentelemetry.io/otel/propagation"
)

// name is the Tracer name used to identify this instrumentation library.
const name = "fib"

// App is an Fibonacci computation application.
type App struct {
	r io.Reader
	l *log.Logger
}

// NewApp returns a new App.
func NewApp(r io.Reader, l *log.Logger) *App {
	return &App{r: r, l: l}
}

// Run starts polling users for Fibonacci number requests and writes results.
func (a *App) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	done := a.waitTerminateSignal()
	breakFlag := false
	for {
		if breakFlag {
			break
		}

		// Each execution of the run loop, we should get a new "root" span and context.
		newCtx, span := otel.Tracer(name).Start(ctx, "Run")
		wg.Add(2)
		go func() {
			defer wg.Done()
			n, err := a.Poll(newCtx)
			if err != nil {
				a.l.Println("Poll failed: ", err)
				span.End()
				return
			}
			a.Write(newCtx, n)
		}()

		go func() {
			wg.Done()
			err := a.HeaderInspect(newCtx)
			if err != nil {
				a.l.Println("HeaderInspect failed: ", err)
				span.End()
				return
			}
		}()

		span.End()
		wg.Wait()

		select {
		case <-done:
			breakFlag = true
		default:
			fmt.Println("Make other span")
		}

	}
	return nil
}

// Poll asks a user for input and returns the request.
func (a *App) Poll(ctx context.Context) (uint, error) {
	_, span := otel.Tracer(name).Start(ctx, "Poll")
	defer span.End()

	a.l.Print("What Fibonacci number would you like to know: ")

	var n uint
	_, err := fmt.Fscanf(a.r, "%d", &n)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}

	// Store n as a string to not overflow an int64.
	nStr := strconv.FormatUint(uint64(n), 10)
	span.SetAttributes(attribute.String("request.n", nStr))

	return n, nil
}

// Write writes the n-th Fibonacci number back to the user.
func (a *App) Write(ctx context.Context, n uint) {
	var span trace.Span
	ctx, span = otel.Tracer(name).Start(ctx, "Write")
	defer span.End()

	f, err := func(ctx context.Context) (uint64, error) {
		_, span := otel.Tracer(name).Start(ctx, "Fibonacci")
		defer span.End()
		f, err := Fibonacci(n)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.AddEvent("Write some log", trace.WithAttributes(attribute.Int("pid", 4321), attribute.String("signal", "SIGHUP")))
		}
		return f, err
	}(ctx)
	if err != nil {
		a.l.Printf("Fibonacci(%d): %v\n", n, err)
	} else {
		a.l.Printf("Fibonacci(%d) = %d\n", n, f)
	}
}

// HeaderInspect check propagation header.
func (a *App) HeaderInspect(ctx context.Context) error {
	var span trace.Span
	_, span = otel.Tracer(name).Start(ctx, "HeaderInspect")
	defer span.End()

	resp, err := http.Get("https://httpbin.org/headers")
	if err != nil {
		a.l.Printf("call http fail")
		return err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		a.l.Printf("body read fail")
		return err
	}
	//Convert the body to type string
	sb := string(body)
	a.l.Printf(sb)
	return nil
}

// waitTerminateSignal ...
func (a *App) waitTerminateSignal() (done chan bool) {

	sigs := make(chan os.Signal, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	done = make(chan bool, 1)

	go func() {
		sig := <-sigs
		a.l.Println("Byeee", sig)
		done <- true
	}()
	return done
}
