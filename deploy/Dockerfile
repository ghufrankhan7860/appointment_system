FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Empty for production. make up passes "swagger" to include the Swagger UI.
ARG GO_BUILD_TAGS=
RUN CGO_ENABLED=0 GOOS=linux go build -tags "${GO_BUILD_TAGS}" -trimpath -ldflags="-s -w" \
    -o /out/appointment-api .

# Uploaded medical files live here with the local storage provider; it belongs
# to the nonroot user so the API can write to it and to a volume mounted on it.
RUN mkdir -p /out/storage

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=builder /out/appointment-api /app/appointment-api
COPY --from=builder --chown=nonroot:nonroot /out/storage /app/var/storage

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/app/appointment-api"]