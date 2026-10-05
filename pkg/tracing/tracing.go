package tracing

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer

func Init(ctx context.Context, serviceName, otlpEndpoint string) (func(context.Context) error, error) {
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(otlpEndpoint), otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(serviceName)))
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(res))
	otel.SetTracerProvider(provider)
	tracer = provider.Tracer(serviceName)
	return provider.Shutdown, nil
}

func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

func RecordLLMSpanAttributes(span trace.Span, model, provider string, inputTokens, outputTokens int, latencyMs int64, finishReason string) {
	span.SetAttributes(
		attribute.String("llm.model", model),
		attribute.String("llm.provider", provider),
		attribute.Int("llm.input_tokens", inputTokens),
		attribute.Int("llm.output_tokens", outputTokens),
		attribute.Int64("llm.latency_ms", latencyMs),
		attribute.String("llm.finish_reason", finishReason),
	)
}

func RecordRetrievalSpanAttributes(span trace.Span, chunkIDs []string, scores []float64, latencyMs int64) {
	span.SetAttributes(
		attribute.StringSlice("retrieval.chunk_ids", chunkIDs),
		attribute.Float64Slice("retrieval.scores", scores),
		attribute.Int64("retrieval.latency_ms", latencyMs),
	)
}
