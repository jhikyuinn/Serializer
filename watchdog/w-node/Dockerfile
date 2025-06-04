# Stage 1: Dependencies만 설치 (캐시용)
FROM golang:1.23-bullseye AS deps

WORKDIR /app

COPY go.mod go.sum ./
COPY mp2btp-main ./mp2btp-main

ENV GOPROXY=direct

RUN go mod download


# Stage 2: Build 및 실행 환경
FROM golang:1.23-bullseye

RUN apt-get update && apt-get install -y \
    gnupg \
    curl \
    git \
    netcat \
    jq \
    iptables

RUN curl -fsSL https://pgp.mongodb.com/server-6.0.asc | gpg --dearmor -o /usr/share/keyrings/mongodb-server-6.0.gpg && \
    echo "deb [ signed-by=/usr/share/keyrings/mongodb-server-6.0.gpg ] https://repo.mongodb.org/apt/debian bullseye/mongodb-org/6.0 main" | tee /etc/apt/sources.list.d/mongodb-org-6.0.list && \
    apt-get update && apt-get install -y mongodb-org

RUN mkdir -p /data/db

WORKDIR /app

# deps 스테이지에서 다운받은 모듈 복사
COPY --from=deps /go/pkg /go/pkg
COPY --from=deps /app/mp2btp-main ./mp2btp-main

COPY go.mod go.sum ./

COPY . .

RUN go build -o wnode  -mod=readonly  ./transceiver/node/main.go 

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

CMD ["/entrypoint.sh"]