FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -trimpath -o /acapulko .

FROM alpine:3
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /acapulko /acapulko
COPY templates/ /templates/
COPY static/ /static/
WORKDIR /
ENTRYPOINT ["/acapulko"]
