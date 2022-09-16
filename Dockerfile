FROM golang:1.18-alpine AS build_deps

RUN apk update && \
    apk upgrade && \
    apk add --no-cache git

WORKDIR /workspace
ENV GO111MODULE=on

COPY go.mod .
COPY go.sum .

RUN go mod download

FROM build_deps AS build

COPY . .

RUN CGO_ENABLED=0 go build -o webhook -ldflags '-s -w -extldflags "-static"' .

FROM alpine:3.16

RUN apk update && \
    apk upgrade && \
    apk add --no-cache ca-certificates && \
    rm -rf /var/cache/apk/*

COPY --from=build /workspace/webhook /usr/local/bin/webhook
RUN setcap cap_net_bind_service=+ep /usr/local/bin/webhook

ENTRYPOINT ["webhook"]
