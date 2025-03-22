package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

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
			semconv.ServiceNameKey.String("monitoring-test-sender"), // 서비스 이름 변경
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
	tracer = tp.Tracer("monitoring-test-sender") // tracer 이름 변경

	return tp, nil
}

// 주기적인 더미 요청 생성을 위한 함수 추가
func startPeriodicRequests(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ticker.C:
				generateDummyTraces()
			}
		}
	}()
	log.Printf("주기적인 더미 요청 생성기가 시작되었습니다 (간격: %v)", interval)
}

// 다양한 엔드포인트에 더미 요청을 보내는 함수
func generateDummyTraces() {
	ctx := context.Background()
	_, span := tracer.Start(ctx, "periodic-dummy-request")
	defer span.End()

	// receiver 주소 가져오기
	receiverEndpoint := os.Getenv("RECEIVER_ENDPOINT")
	if receiverEndpoint == "" {
		receiverEndpoint = "http://localhost:8081" // 기본값
		log.Println("RECEIVER_ENDPOINT 환경 변수가 설정되지 않았습니다. 기본값 http://localhost:8081을 사용합니다.")
	}

	endpoints := []string{"/", "/health"} // receiver의 엔드포인트만 사용

	// 무작위 엔드포인트 선택
	endpoint := endpoints[rand.Intn(len(endpoints))]

	// 내부적으로 HTTP 요청 생성
	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	reqURL := fmt.Sprintf("%s%s", receiverEndpoint, endpoint) // receiver 주소 사용
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		log.Printf("더미 요청 생성 실패: %v", err)
		return
	}

	span.SetAttributes(attribute.String("dummy.request.url", reqURL))
	span.SetAttributes(attribute.String("dummy.request.type", "periodic"))

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("더미 요청 실패: %v", err)
		return
	}
	defer resp.Body.Close()

	log.Printf("더미 요청 완료: %s, 상태: %d", endpoint, resp.StatusCode)
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

	// 주기적인 더미 요청 시작 (5초마다)
	startPeriodicRequests(5 * time.Second)

	// 서버 시작 X (sender는 더 이상 HTTP 서버가 아님)
	log.Println("sender 시작됨. receiver로 요청 전송.")

	// 대기
	for {
	}
}

func init() {
	// 난수 생성기 초기화
	rand.Seed(time.Now().UnixNano())
}
