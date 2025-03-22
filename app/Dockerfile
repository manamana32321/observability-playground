# 빌드 스테이지: Go 애플리케이션 빌드
FROM golang:1.20-alpine AS builder

# 작업 디렉토리 설정
WORKDIR /app

# 필요한 파일 복사
COPY go.* ./
# go.mod 및 go.sum이 있는 경우 종속성 다운로드
RUN go mod download || true

# 소스 코드 복사
COPY *.go ./

# 애플리케이션 빌드
RUN CGO_ENABLED=0 GOOS=linux go build -o monitoring-server .

# 실행 스테이지: 최소한의 이미지로 실행
FROM alpine:3.17

# 필수 CA 인증서 설치
RUN apk --no-cache add ca-certificates

# 작업 디렉토리 설정
WORKDIR /app

# 빌드 스테이지에서 컴파일된 바이너리 복사
COPY --from=builder /app/monitoring-server .

# 컨테이너 시작 시 실행할 명령어
ENTRYPOINT ["./monitoring-server"]

# 포트 노출
EXPOSE 8080

# 애플리케이션이 컨테이너 내에서 실행될 때 사용할 사용자 지정
USER nobody