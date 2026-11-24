# Media Processing Service

Практична робота №7: gRPC-мікросервіс для реєстрації завдань обробки медіа, перевірки їхнього стану та потокового відстеження прогресу.

## Реалізовано

- Контракт Protocol Buffers у `proto/media.proto`.
- Unary RPC: `SubmitJob`, `GetJobStatus`.
- Server Streaming RPC: `TrackProgress`.
- Потокобезпечне in-memory сховище завдань.
- Unary interceptor для перевірки `x-api-key`, логування клієнта, методу та тривалості виклику.
- CLI-клієнт: негативна перевірка автентифікації, створення завдання, читання потоку та фінальна перевірка стану.

## Вимоги

- Go 1.22+
- `protoc`
- `protoc-gen-go`
- `protoc-gen-go-grpc`

## Встановлення залежностей і генераторів

```bash
go mod tidy
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
export PATH="$(go env GOPATH)/bin:$PATH"
```

## Генерація Go-коду з контракту

```bash
protoc --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  proto/media.proto
```

## Запуск

У першому терміналі:

```bash
go run ./cmd/server
```

У другому терміналі:

```bash
go run ./cmd/client
```

Сервер слухає `:50051`. API-ключ: `secret-media-token`. Він передається як gRPC Metadata `x-api-key`.

## Перевірка

```bash
go test ./...
```

Клієнт навмисно надсилає перший unary-запит без Metadata та очікує код `Unauthenticated`. Після цього він створює завдання з коректним ключем, отримує чотири потокові оновлення прогресу та перевіряє фінальний стан `COMPLETED`.

## Структура

```text
cmd/client/main.go   CLI-клієнт
cmd/server/main.go   gRPC-сервер
proto/media.proto    Protobuf-контракт
proto/media.pb.go    згенеровані повідомлення
proto/media_grpc.pb.go згенеровані gRPC-стаби
```
