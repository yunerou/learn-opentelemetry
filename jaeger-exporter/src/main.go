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
	"log"
	"os"
	"os/signal"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"

	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/propagation"
	controller "go.opentelemetry.io/otel/sdk/metric/controller/basic"
	processor "go.opentelemetry.io/otel/sdk/metric/processor/basic"
	"go.opentelemetry.io/otel/sdk/metric/selector/simple"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	service     = "server-demo"
	environment = "test"
	id          = 1
)

func initTraceProvider(ctx context.Context) (func(), error) {
	otelAgentAddr, ok := os.LookupEnv("OTEL_EXPORTER_JAEGER_ENDPOINT")
	fmt.Println(otelAgentAddr)
	if !ok {
		otelAgentAddr = "0.0.0.0:4317"
	}
	traceResource := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(service),
		attribute.String("environment", environment),
		attribute.Int64("ID", id),
	)

	exporter, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint("http://" + otelAgentAddr + "/api/traces")))
	if err != nil {
		log.Fatalf("%s: %v", "Can not connect Jaeger address", err)
		return func() {}, nil
	}

	tracerProvider := sdktrace.NewTracerProvider(
		// Always be sure to batch in production.
		sdktrace.WithBatcher(exporter),
		// Record information about this application in a Resource.
		sdktrace.WithResource(traceResource),
	)

	// set global propagator to tracecontext (the default is no-op).
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetTracerProvider(tracerProvider)

	return func() {
		cxt, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if err := exporter.Shutdown(cxt); err != nil {
			otel.Handle(err)
		}
	}, nil
}

func initMetricProvider(ctx context.Context) func() {
	otelAgentAddr, ok := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if !ok {
		otelAgentAddr = "0.0.0.0:4317"
	}

	metricClient := otlpmetricgrpc.NewClient(
		otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithEndpoint(otelAgentAddr))
	metricExp, err := otlpmetric.New(ctx, metricClient)
	if err != nil {
		log.Fatalf("%s: %v", "Failed to create the collector metric exporter", err)
	}

	pusher := controller.New(
		processor.NewFactory(
			simple.NewWithHistogramDistribution(),
			metricExp,
		),
		controller.WithExporter(metricExp),
		controller.WithCollectPeriod(2*time.Second),
	)
	global.SetMeterProvider(pusher)

	err = pusher.Start(ctx)
	if err != nil {
		log.Fatalf("%s: %v", "Failed to start metric pusher", err)
	}
	return func() {
		cxt, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		// pushes any last exports to the receiver
		if err := pusher.Stop(cxt); err != nil {
			otel.Handle(err)
		}
	}
}

func main() {
	ctx := context.Background()
	shutdown, err := initTraceProvider(ctx)
	if err != nil {
		panic("Can not start initTraceProvider")
	}
	defer shutdown()

	l := log.New(os.Stdout, "", 0)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	errCh := make(chan error)
	app := NewApp(os.Stdin, l)
	go func() {
		errCh <- app.Run(context.Background())
	}()

	select {
	case <-sigCh:
		l.Println("\ngoodbye")
		return
	case err := <-errCh:
		if err != nil {
			l.Fatal(err)
		}
	}
}
