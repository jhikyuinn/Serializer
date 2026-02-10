# ------------------ Build Stage ------------------
    FROM golang:1.23-bullseye AS builder

    # 필수 패키지 설치
    RUN apt-get update && apt-get install -y \
        build-essential \
        cmake \
        git \
        wget \
        ca-certificates \
        libgmp-dev \
        libmpfr-dev \
        && rm -rf /var/lib/apt/lists/*
    
    # 작업 디렉토리 설정
    WORKDIR /app
    COPY . .
    
    # validator 폴더로 이동 후 의존성 처리
    WORKDIR /app/validator
    RUN go mod tidy
    RUN go mod vendor
    
    # ---------- Install MCL ----------
    WORKDIR /tmp
    RUN git clone https://github.com/herumi/mcl.git && \
        cd mcl && \
        cmake -DBUILD_SHARED_LIBS=ON . && \
        make && make install
    
    WORKDIR /app/validator
    
    # ---------- Fix go-libp2p-pubsub Pretty -> String ----------
    RUN sed -i 's/p.Pretty()/p.String()/g' ../vendor/github.com/libp2p/go-libp2p-pubsub/gossipsub.go
    
    # ---------- Set CGO flags for BLS ----------
    ENV CGO_ENABLED=1
    ENV CGO_CFLAGS="-I/usr/local/include -I/app/validator/vendor/github.com/herumi/bls-eth-go-binary/bls/include"
    ENV CGO_LDFLAGS="-L/usr/local/lib"
    
    ENV GOFLAGS=-mod=vendor
    
    # ---------- Build validator ----------
    RUN go build -o validator ./main.go
    
    # ------------------ Runtime Stage ------------------
    FROM debian:bullseye
    
    # 런타임 필수 패키지 설치
    RUN apt-get update && apt-get install -y \
        ca-certificates \
        libssl-dev \
        libgmp-dev \
        libmpfr-dev \
        && rm -rf /var/lib/apt/lists/*
    
    WORKDIR /app
    
    # 빌드한 바이너리와 config 복사
    COPY --from=builder /app/validator/validator .
    COPY --from=builder /app/validator/config ./config
    
    # MCL 공유 라이브러리 복사
    COPY --from=builder /usr/local/lib/libmcl.so /usr/local/lib/
    COPY --from=builder /usr/local/lib/libmcl.so.0 /usr/local/lib/
    
    # 런타임 라이브러리 경로 설정
    ENV LD_LIBRARY_PATH=/usr/local/lib
    
    # 컨테이너 실행 시 validator 실행
    CMD ["./validator"]
    