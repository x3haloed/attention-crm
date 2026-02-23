FROM node:22-alpine AS assets
WORKDIR /src
COPY package.json package-lock.json tailwind.config.js ./
COPY web ./web
COPY internal ./internal
COPY docs/design ./docs/design
RUN npm ci && npm run build:css

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=assets /src/web/static/tailwind.css ./web/static/tailwind.css
RUN go build -trimpath -ldflags="-s -w" -o /out/attention ./cmd/attention

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/attention ./attention
EXPOSE 8080
CMD ["./attention"]
