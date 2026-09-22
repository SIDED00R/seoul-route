# 빌드: 레포 루트에서 `docker build -f backend/Dockerfile -t seoul-route-api backend`
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# GIT_SHA: 빌드한 커밋. /health 의 version 으로 나가 배포 스모크(deploy/smoke.sh)가 체크아웃 커밋과 대조한다.
ARG GIT_SHA=unknown
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${GIT_SHA}" -o /out/api ./cmd/api

# distroless nonroot: 셸 없음, uid 65532
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
EXPOSE 8081
ENTRYPOINT ["/api"]
