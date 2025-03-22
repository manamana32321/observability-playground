package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2" // EC2 서비스 클라이언트 임포트
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.7.0"
	"go.opentelemetry.io/otel/trace"

	otelaws "go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws" // AWS 계측 추가
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
			semconv.ServiceNameKey.String("monitoring-test-app"),
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
	tracer = tp.Tracer("monitoring-test-app")

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

	endpoints := []string{"/", "/health", "/slow", "/error"}

	// 무작위 엔드포인트 선택
	endpoint := endpoints[rand.Intn(len(endpoints))]

	// 내부적으로 HTTP 요청 생성
	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	reqURL := fmt.Sprintf("http://localhost:8080%s", endpoint)
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

	// AWS SDK 클라이언트 생성 및 계측 (EC2 예시)
	awsCfg, awsErr := config.LoadDefaultConfig(context.TODO())
	if awsErr != nil {
		log.Printf("AWS 설정 로드 실패: %v", awsErr)
	} else {
		ec2Client := ec2.NewFromConfig(awsCfg, func(o *ec2.Options) {
			otelaws.AppendMiddlewares(&o.APIOptions)
		})
		log.Printf("EC2 클라이언트가 AWS 계측으로 설정되었습니다: %v", ec2Client)
		// 이제 ec2Client를 사용하여 AWS EC2와 통신하면 트레이싱 정보가 자동으로 포함됩니다.
		_, err = ec2Client.DescribeInstances(context.TODO(), nil)
		if err != nil {
			log.Printf("EC2 인스턴스 정보 조회 실패: %v", err)
		}
	}

	// 주기적인 더미 요청 시작 (5초마다)
	startPeriodicRequests(5 * time.Second)

	// 핸들러를 OpenTelemetry로 감싸기
	http.Handle("/", otelhttp.NewHandler(http.HandlerFunc(homeHandler), "home"))
	http.Handle("/health", otelhttp.NewHandler(http.HandlerFunc(healthHandler), "health"))
	http.Handle("/slow", otelhttp.NewHandler(http.HandlerFunc(slowResponseHandler), "slow"))
	http.Handle("/error", otelhttp.NewHandler(http.HandlerFunc(errorHandler), "error"))

	// 서버 시작
	port := 8080
	log.Printf("서버가 포트 %d에서 시작됩니다...", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Fatalf("서버 시작 실패: %v", err)
	}
}

// 기본 홈페이지 핸들러
func homeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracer.Start(ctx, "home-handler")
	defer span.End()

	log.Printf("홈페이지 요청: %s %s", r.Method, r.URL.Path)
	span.SetAttributes(attribute.String("http.method", r.Method))

	fmt.Fprintf(w, "Hello, World! 모니터링 테스트 서버입니다.\n")
}

// 상태 확인 핸들러
func healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span := tracer.Start(ctx, "health-handler")
	defer span.End()

	log.Printf("상태 확인 요청: %s %s", r.Method, r.URL.Path)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "상태: 정상\n")
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
