# Hướng dẫn phát triển backend

Tài liệu này quy định cách triển khai. Nghiệp vụ, phạm vi và quyết định nhóm nằm trong [contract](db-schema-and-api-contract.md); API chuẩn ở [`api/openapi.yaml`](../api/openapi.yaml). Không sao chép business rules sang hướng dẫn agent.

## Kiến trúc

Clean Architecture có chọn lọc, phụ thuộc `adapter → app → domain`:

```text
cmd/api, cmd/worker          composition roots, khai báo graph trực tiếp bằng Uber Fx
internal/<module>/
  domain/                   entity, quy tắc, lỗi nghiệp vụ
  app/                      use case và ports.go
  adapter/http/             handler, DTO, map lỗi sang HTTP
  adapter/postgres/         implement port bằng sqlc, map row sang entity
internal/shared/kernel/     kiểu dùng chung và Clock
internal/platform/          config, PostgreSQL, HTTP server, lifecycle
internal/db/                code sqlc sinh tự động
```

Đây là cấu trúc đích; xem README để biết phần đã triển khai. Không tạo layer/interface rỗng.

- `event`, `registration`: đủ domain/app/adapter. Check-in thuộc registration.
- `auth`, `unit`, `notification`, `activitylog`, `report`: app mỏng và adapter, không bắt buộc domain. Report được dùng query đọc trực tiếp.
- Domain chỉ import stdlib và shared/kernel. Domain/app không import Echo, pgx, internal/db, net/http hoặc Fx.
- Interface đặt ở phía dùng (`app/ports.go`), chỉ tạo khi cần abstraction/thay thế.
- Use case trả lỗi nghiệp vụ; adapter HTTP mới map status và mã lỗi theo OpenAPI.
- Domain/app lấy thời gian qua `kernel.Clock` để test được các biên thời gian.

## Dependency injection và lifecycle

Hạ tầng cung cấp `fx.Module`, constructor và lifecycle hook. Fx quản lý startup/shutdown; constructor/hook trả lỗi thay vì Fatal/os.Exit. HTTP và worker dừng trước DB pool. Không tạo package bootstrap riêng. Dependency mới phải có lý do trong PR.

## Database và transaction

- Schema thực thi: `db/migrations/*.sql`. Không sửa migration đã merge; thêm migration mới có up/down. Đồng bộ DDL giải thích trong contract khi đổi schema.
- Viết SQL trong `db/queries/`, chạy `make sqlc`; không sửa tay `internal/db/`, không dùng ORM.
- Adapter dùng `internal/platform/postgres/transaction.go` để quản lý begin/commit/rollback, bind repository qua sqlc `WithTx(tx)`.
- App chỉ thấy port UnitOfWork/repository, không thấy `pgx.Tx`. Mọi repository trong một UnitOfWork phải dùng cùng transaction; không vô tình gọi query qua pool.
- Không giữ transaction để gọi email/HTTP bên ngoài. Quy tắc khóa event, counter, outbox và atomicity theo contract mục 6; đọc mục này trước mọi thay đổi chỗ/check-in/job.
- Thiết lập Podman, migration, seed và integration test: [`db/README.md`](../db/README.md).

## API và bảo mật

Đổi path, request/response hoặc mã lỗi thì cập nhật `api/openapi.yaml` trước hoặc cùng thay đổi. Không tự thêm mã ngoài enum ErrorCode. Quy ước payload, phân trang, version và quyền xem dữ liệu theo contract mục 4–6.

Không commit secret hoặc dữ liệu sinh viên thật. Không log token, email, MSSV hoặc thông tin kết nối nhạy cảm. Auth phải theo contract mục 5.1, không suy diễn quyền từ email/client.

## Kiểm tra trước khi hoàn tất

```sh
make sqlc       # nếu thay đổi SQL/schema ảnh hưởng generated code
make lint      # gofmt check + go vet
make test      # unit tests, có race detector
make test-int  # thay đổi DB/transaction/event/registration; cần TEST_DATABASE_URL
```

- Domain: table-driven tests với clock giả, không DB.
- App: fake repository/ports để kiểm tra use case.
- Integration: build tag `integration`, PostgreSQL thật; hướng dẫn quyền CREATEDB và database cô lập ở `db/README.md`.
- Test nghiệm thu/concurrency bắt buộc khi triển khai nghiệp vụ theo contract mục 7: đăng ký đồng thời, FIFO, check-in trùng và phân quyền khác đơn vị.
- Nếu chưa chạy được test nào, báo rõ điều kiện thiếu; không coi compile/pass unit test là đã kiểm chứng nghiệp vụ.

## Giữ tài liệu nhất quán

README ghi trạng thái triển khai thực tế, contract ghi thiết kế. Không mô tả chức năng dự kiến như đã có. Chỉ dùng đường dẫn chuẩn, không giữ bản sao hậu tố tải xuống. Các quyết định nhóm chưa được client xác nhận phải tiếp tục được đánh dấu trong contract.
