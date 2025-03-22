package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"context"
	"math/rand"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer

func initTracer() (*sdktrace.TracerProvider, error) {
	// OTLP exporter 생성
	ctx := context.Background()

	// Tempo 서버로 전송
	tempoEndpoint := os.Getenv("TEMPO_ENDPOINT")
	if tempoEndpoint == "" {
		tempoEndpoint = "tempo:4317" // 기본값
	}

	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint(tempoEndpoint),
		otlptracegrpc.WithInsecure(), // 테스트 환경에서는 TLS 없이 설정
	)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("OTLP exporter 생성 실패: %w", err)
	}

	// 리소스 설정 (서비스 이름 등)
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("monitoring-test-receiver"),
			attribute.String("environment", "dev"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("리소스 생성 실패: %w", err)
	}

	// TracerProvider 설정
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	// 글로벌 tracer 설정
	tracer = tp.Tracer("monitoring-test-receiver")

	return tp, nil
}

func main() {
	// 트레이서 초기화
	tp, err := initTracer()
	if err != nil {
		log.Fatalf("트레이서 초기화 실패: %v", err)
	}
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}()

	// 핸들러를 OpenTelemetry로 감싸기
	http.Handle("/", otelhttp.NewHandler(http.HandlerFunc(homeHandler), "home"))
	http.Handle("/health", otelhttp.NewHandler(http.HandlerFunc(healthHandler), "health"))
	http.Handle("/slow", otelhttp.NewHandler(http.HandlerFunc(slowResponseHandler), "slow"))
	http.Handle("/error", otelhttp.NewHandler(http.HandlerFunc(errorHandler), "error"))

	// 서버 시작
	port := 8081 // sender와 다른 포트 사용
	log.Printf("수신 서버가 포트 %d에서 시작됩니다...", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Fatalf("수신 서버 시작 실패: %v", err)
	}
}

// 기본 홈페이지 핸들러
func homeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracer.Start(ctx, "home-handler")
	defer span.End()

	log.Printf("수신: 홈페이지 요청: %s %s", r.Method, r.URL.Path)
	span.SetAttributes(attribute.String("http.method", r.Method))

	fmt.Fprintf(w, "수신 서버: Hello, World!\n")
}

// 상태 확인 핸들러
func healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracer.Start(ctx, "health-handler")
	defer span.End()

	log.Printf("수신: 상태 확인 요청: %s %s", r.Method, r.URL.Path)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "수신 서버: 상태: 정상\n")
}

// 느린 응답을 생성하는 핸들러
func slowResponseHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracer.Start(ctx, "slow-handler")
	defer span.End()

	log.Printf("느린 응답 요청: %s %s", r.Method, r.URL.Path)

	// 0.1초에서 2초 사이의 무작위 지연
	delay := 100 + rand.Intn(1900)
	span.SetAttributes(attribute.Int("delay_ms", delay))

	time.Sleep(time.Duration(delay) * time.Millisecond)

	fmt.Fprintf(w, "느린 응답 완료! 지연 시간: %d ms\n", delay)
}

// 에러를 발생시키는 핸들러
func errorHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracer.Start(ctx, "error-handler")
	defer span.End()

	log.Printf("에러 발생 요청: %s %s", r.Method, r.URL.Path)

	// 20% 확률로 500 에러 반환
	if rand.Intn(5) == 0 {
		log.Printf("500 에러 발생")
		span.SetAttributes(attribute.String("error", "true"))
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "내부 서버 오류가 발생했습니다!\n")
		return
	}

	// 80% 확률로 정상 응답
	fmt.Fprintf(w, "이번에는 에러가 발생하지 않았습니다!\n")
}

func init() {
	// 난수 생성기 초기화
	rand.Seed(time.Now().UnixNano())
}
