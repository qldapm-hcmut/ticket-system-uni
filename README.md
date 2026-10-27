# University Event Registration Backend

Backend hệ thống đăng ký sự kiện trường đại học — CO3011 · L01 · Nhóm 09.

## Stack

Go **1.27+**, Echo v4, PostgreSQL 16, pgx/v5. Index cho agent: [`docs/AGENTS.md`](docs/AGENTS.md). Kiến trúc và hướng dẫn phát triển: [`docs/development.md`](docs/development.md). Contract: [`api/openapi.yaml`](api/openapi.yaml). Thiết kế dữ liệu: [`docs/db-schema-and-api-contract.md`](docs/db-schema-and-api-contract.md).

## Chạy local

Yêu cầu: Go 1.27+ (hoặc Go hỗ trợ tự tải toolchain), Podman với Compose provider (`podman compose`), Make.

```sh
cp .env.example .env
make db-up
make run
# Terminal khác:
curl http://localhost:3000/health/live
curl http://localhost:3000/health/ready
make worker
```

Make tự nạp `.env` và mặc định `GOTOOLCHAIN=auto`. Khi dùng `go run` trực tiếp, cần tự export biến môi trường và chọn đúng toolchain. Không commit `.env` hoặc secret. Database local chỉ bind loopback; mật khẩu ví dụ không dành cho production. Nếu Compose provider cần Podman socket, chạy `systemctl --user start podman.socket` trước `make db-up`. `podman compose` có thể sử dụng plugin docker-compose làm provider nhưng container vẫn chạy bằng Podman, không cần Docker daemon.

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

## Baseline tài liệu

Baseline thiết kế ngày **24/10/2026**, API **1.1.0**:

- [`api/openapi.yaml`](api/openapi.yaml): API contract chuẩn cho backend/frontend.
- [`docs/db-schema-and-api-contract.md`](docs/db-schema-and-api-contract.md): nghiệp vụ, mô hình dữ liệu, phân quyền và tiêu chí nghiệm thu.
- [`docs/AGENTS.md`](docs/AGENTS.md): index tra cứu theo công việc cho agent.
- [`docs/development.md`](docs/development.md): kiến trúc, transaction, quy tắc triển khai và kiểm tra.
- [`db/migrations/`](db/migrations/): schema thực thi; [`db/README.md`](db/README.md) ghi cách dùng và phần chưa có.

Vai trò chồng lấn (`roles[]`), Admin cấp trường, MSSV từ import, ngoại lệ hủy khi đổi giờ/địa điểm trong 24h cuối và khóa sửa giờ/địa điểm 60 phút cuối là baseline mới. Các quyết định nhóm còn cần xác nhận được ghi rõ trong contract; không coi việc đồng bộ tài liệu là phê duyệt của client.

## Trạng thái skeleton

`cmd/api/main.go` và `cmd/worker/main.go` là composition roots: khai báo dependency graph trực tiếp bằng Uber Fx, không có package bootstrap. Mỗi dependency cung cấp `Module` với lifecycle hooks. Fx start theo thứ tự dependency và stop ngược lại: HTTP/worker dừng trước khi Postgres đóng. Constructor/hook trả lỗi startup để Fx log; không log lặp ở dependency. Lỗi HTTP xảy ra sau startup được log rồi yêu cầu Fx shutdown với exit code 1.

Dependency mới `go.uber.org/fx` dùng cho dependency injection và lifecycle; không thêm framework nội bộ, Redis hay tracing. Fx quản lý signal và timeout start/stop.

Đã có config môi trường, pgxpool, HTTP middleware/error envelope, health probes và shared Clock. Dependencies trực tiếp gồm Echo (HTTP/middleware), pgx (PostgreSQL) và Uber Fx (DI/lifecycle).

**Đã bổ sung schema:** initial migration up/down theo baseline mới; đã kiểm chứng up/down/up trên PostgreSQL 16 chạy bằng Podman.

**Đã bổ sung database foundation:** sqlc v1.30.0, query user/identity/quyền, generated `internal/db`, seed bootstrap đơn vị/Admin và integration test migration/constraint trên database cô lập. Xem `db/README.md` để cài CLI và chạy. Đã chạy integration test trên PostgreSQL 16 bằng Podman và kiểm tra seed chạy hai lần không tạo trùng user/audit.

**Chưa triển khai:** import MSSV, Google OAuth/OIDC, JWT/session, các module nghiệp vụ, email và jobs. Worker chỉ kết nối DB và chờ tín hiệu dừng. `make test-int` cần `TEST_DATABASE_URL` và role có CREATEDB; migrate/sqlc/seed cần CLI tương ứng. Không có endpoint login mật khẩu hay tài khoản demo.

Module `event` và `registration` sẽ tách `domain / app / adapter`; các module còn lại thêm khi có use case thật, không tạo lớp/interface rỗng. Không chỉnh tay code sqlc khi bắt đầu sinh `internal/db`.
