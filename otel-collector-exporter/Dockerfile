FROM golang:1.17-buster AS base
WORKDIR /app
RUN go env -w GOPROXY=direct

FROM base as dev
COPY src /app
RUN go mod tidy
CMD ["go", "run","."]
EXPOSE 7080


FROM base as builder
COPY src/go.mod ./
COPY src/go.sum ./
RUN go mod download
COPY *.go ./
RUN go build -o /server-demo


FROM gcr.io/distroless/base-debian10 as deployer
WORKDIR /
COPY --from=builder /server-demo /server-demo
EXPOSE 7080
USER nonroot:nonroot
ENTRYPOINT ["/server-demo"]