FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY core/ ./core/
RUN cd core && go build -o /magic ./cmd/magic

FROM alpine:3.20
RUN addgroup -S magic && adduser -S magic -G magic
COPY --from=builder /magic /usr/local/bin/magic
USER magic
EXPOSE 8080
ENV MAGIC_PORT=8080
ENTRYPOINT ["magic", "serve"]
