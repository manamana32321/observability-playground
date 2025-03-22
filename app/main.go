package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"
)

func main() {
	// 기본 핸들러 설정
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/slow", slowResponseHandler)
	http.HandleFunc("/error", errorHandler)

	// 서버 시작
	port := 8080
	log.Printf("서버가 포트 %d에서 시작됩니다...", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Fatalf("서버 시작 실패: %v", err)
	}
}

// 기본 홈페이지 핸들러
func homeHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("홈페이지 요청: %s %s", r.Method, r.URL.Path)
	fmt.Fprintf(w, "Hello, World! 모니터링 테스트 서버입니다.\n")
}

// 상태 확인 핸들러
func healthHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("상태 확인 요청: %s %s", r.Method, r.URL.Path)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "상태: 정상\n")
}

// 느린 응답을 생성하는 핸들러
func slowResponseHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("느린 응답 요청: %s %s", r.Method, r.URL.Path)

	// 0.1초에서 2초 사이의 무작위 지연
	delay := 100 + rand.Intn(1900)
	time.Sleep(time.Duration(delay) * time.Millisecond)

	fmt.Fprintf(w, "느린 응답 완료! 지연 시간: %d ms\n", delay)
}

// 에러를 발생시키는 핸들러
func errorHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("에러 발생 요청: %s %s", r.Method, r.URL.Path)

	// 20% 확률로 500 에러 반환
	if rand.Intn(5) == 0 {
		log.Printf("500 에러 발생")
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
