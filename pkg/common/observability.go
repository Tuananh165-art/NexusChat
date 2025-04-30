package common

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/minghsu0107/go-random-chat/pkg/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
)

var TracerProvider *tracesdk.TracerProvider

type ObservabilityInjector struct {
	promPort     string
	otlpEndpoint string
}

func NewObservabilityInjector(config *config.Config) *ObservabilityInjector {
	return &ObservabilityInjector{
		promPort:     config.Observability.Prometheus.Port,
		otlpEndpoint: config.Observability.Tracing.OtlpEndpoint,
	}
}

func (injector *ObservabilityInjector) Register(service string) error {
	if injector.otlpEndpoint != "" {
		err := initTracerProvider(injector.otlpEndpoint, service)
		if err != nil {
			return err
		}
		otel.SetTracerProvider(TracerProvider)
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	}
	if injector.promPort != "" {
		go func() {
			promHttpSrv := &http.Server{Addr: fmt.Sprintf(":%s", injector.promPort)}
			m := http.NewServeMux()
			// Create HTTP handler for Prometheus metrics.
			m.Handle("/metrics", promhttp.HandlerFor(
				prometheus.DefaultGatherer,
				promhttp.HandlerOpts{
					// Opt into OpenMetrics e.g. to support exemplars.
					EnableOpenMetrics: true,
				},
			))
			promHttpSrv.Handler = m
			slog.Info("starting prom metrics on  :" + injector.promPort)
			err := promHttpSrv.ListenAndServe()
			if err != nil {
				slog.Error(err.Error())
				os.Exit(1)
			}
		}()
	}
	return nil
}

func otelReqFilter(req *http.Request) bool {
	filters := []string{"/metrics", "/", "/healthcheck"}
	for _, filter := range filters {
		if filter == req.URL.Path {
			return false
		}
	}
	return true
}

func NewOtelHttpHandler(h http.Handler, operation string) http.Handler {
	httpOptions := []otelhttp.Option{
		otelhttp.WithTracerProvider(otel.GetTracerProvider()),
		otelhttp.WithPropagators(otel.GetTextMapPropagator()),
		otelhttp.WithFilter(otelReqFilter),
	}
	return otelhttp.NewHandler(h, operation, httpOptions...)
}

func initTracerProvider(otlpEndpoint, service string) error {
	ctx := context.Background()

	// Create OTLP exporter
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(otlpEndpoint),
		otlptracehttp.WithInsecure(),
	}
	client := otlptracehttp.NewClient(opts...)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return fmt.Errorf("creating OTLP trace exporter: %w", err)
	}

	TracerProvider = tracesdk.NewTracerProvider(
		tracesdk.WithSampler(tracesdk.ParentBased(tracesdk.TraceIDRatioBased(0.0001))),
		tracesdk.WithBatcher(exporter),
		// Record information about this application in a Resource.
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(service),
		)),
	)
	return nil
}
