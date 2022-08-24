FROM golang:1.19 as build

WORKDIR /work
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY main.go .
RUN go build -o slayer

FROM gcr.io/distroless/base

COPY --from=build /work/slayer /
CMD ["/slayer"]
