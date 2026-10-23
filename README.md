# University Event Registration Backend

Backend hệ thống đăng ký sự kiện trường đại học — CO3011 · L01 · Nhóm 09.

## Stack

Go **1.27+**, Echo v4, PostgreSQL 16, pgx/v5. Kiến trúc và nghiệp vụ: [`docs/AGENTS.md`](docs/AGENTS.md). Contract: [`api/openapi.yaml`](api/openapi.yaml). Thiết kế dữ liệu: [`docs/db-schema-and-api-contract.md`](docs/db-schema-and-api-contract.md).

## Chạy local

Yêu cầu: Go 1.27+ (hoặc Go hỗ trợ tự tải toolchain), Docker Compose, Make.

```sh
cp .env.example .env
make db-up
make run
# Terminal khác:
curl http://localhost:3000/health/live
curl http://localhost:3000/health/ready
make worker
```

Make tự nạp `.env` và mặc định `GOTOOLCHAIN=auto`. Khi dùng `go run` trực tiếp, cần tự export biến môi trường và chọn đúng toolchain. Không commit `.env` hoặc secret. Database local chỉ bind loopback; mật khẩu ví dụ không dành cho production.

- `GET /health/live`: process đang phục vụ HTTP (200).
- `GET /health/ready`: ping PostgreSQL (200 hoặc 503), không trả chi tiết kết nối.
- API và worker dừng qua SIGINT/SIGTERM; API có graceful shutdown.

```sh
make fmt
make lint  # gofmt check + go vet; chưa cài golangci-lint
make test  # unit tests với race detector, không cần DB
make build
make db-down # giữ volume dữ liệu
```

## Neovim / gopls: packages.Load error

Nếu system Go cũ hơn 1.27 và `go env GOTOOLCHAIN` trả `local`, gopls không thể load module. Makefile bật `auto` riêng cho các lệnh Make, không áp dụng cho editor. Bật lựa chọn toolchain ở cấp user:

```sh
go env -w GOTOOLCHAIN=auto
go version
go list ./...
```

Sau đó khởi động lại Neovim/LSP. Nếu shell hoặc cấu hình LSP đặt rõ `GOTOOLCHAIN=local`, cần bỏ override đó vì biến môi trường ưu tiên hơn `go env -w`. Lần đầu tải toolchain cần kết nối mạng. Không hạ Go version trong `go.mod` để né lỗi editor.

## Trạng thái skeleton

`cmd/api/main.go` và `cmd/worker/main.go` là composition roots: khai báo dependency graph trực tiếp bằng Uber Fx, không có package bootstrap. Mỗi dependency cung cấp `Module` với lifecycle hooks. Fx start theo thứ tự dependency và stop ngược lại: HTTP/worker dừng trước khi Postgres đóng. Constructor/hook trả lỗi startup để Fx log; không log lặp ở dependency. Lỗi HTTP xảy ra sau startup được log rồi yêu cầu Fx shutdown với exit code 1.

Dependency mới `go.uber.org/fx` dùng cho dependency injection và lifecycle; không thêm framework nội bộ, Redis hay tracing. Fx quản lý signal và timeout start/stop.

Đã có config môi trường, pgxpool, HTTP middleware/error envelope, health probes và shared Clock. Dependencies trực tiếp gồm Echo (HTTP/middleware), pgx (PostgreSQL) và Uber Fx (DI/lifecycle).

**Chưa triển khai:** migrations/seed/sqlc, Google OAuth/OIDC, JWT/session, các module nghiệp vụ, email và jobs. Worker hiện chỉ kết nối DB và chờ tín hiệu dừng, không giả lập job thành công. `make test-int` chưa có integration test; các target migrate/sqlc/seed chỉ sử dụng được sau khi bổ sung SQL, cấu hình và CLI tương ứng. Không có endpoint login mật khẩu hay tài khoản demo.

Module `event` và `registration` sẽ tách `domain / app / adapter`; các module còn lại thêm khi có use case thật, không tạo lớp/interface rỗng. Không chỉnh tay code sqlc khi bắt đầu sinh `internal/db`.
