# Agent index — University Event Registration Backend

Backend Go cho đăng ký sự kiện trường: tạo/duyệt → đăng ký/waitlist → check-in → báo cáo. Stack hiện có: Echo, Uber Fx, pgx, sqlc, PostgreSQL 16; local dùng Podman.

## Đọc đúng phần cho công việc

Không đọc toàn bộ docs mặc định. Tra heading bằng `rg -n '^##|^###' docs/db-schema-and-api-contract.md`, rồi đọc phần liên quan.

| Công việc | Đọc trước | Code/nguồn cần tìm |
| --- | --- | --- |
| Setup, trạng thái hiện tại | `README.md`, `db/README.md` | `Makefile`, `compose.yaml`, `.env.example` |
| Kiến trúc, repository, transaction, test | `docs/development.md` | `internal/platform/`, `internal/shared/kernel/`, `cmd/` |
| Auth, user, vai trò | Contract §1, §5.1, §6.1 | OpenAPI operation tương ứng; `db/queries/auth.sql` |
| Event, duyệt, sửa/hủy | Contract §3, §5.2–5.3, §6.4 | OpenAPI; `internal/event/` khi triển khai |
| Registration, waitlist, check-in | Contract §5.4–5.5, §6.2–6.5 | `internal/registration/` khi triển khai; transaction runner |
| Notification, worker, report | Contract §5.6–5.7, §6.6 | `cmd/worker/`; module liên quan khi triển khai |
| Schema, query, seed | `db/README.md`, contract §2 | `db/migrations/`, `db/queries/`, `db/seed/`, `sqlc.yaml` |
| Tiêu chí nghiệm thu | Contract §7 | Test module liên quan, `db/schema_integration_test.go` |

Contract là `docs/db-schema-and-api-contract.md`. Module được nhắc trong bảng có thể chưa tồn tại; kiểm tra README và cây file trước khi giả định đã triển khai.

## Nguồn chuẩn

- Nghiệp vụ/phân quyền/phạm vi: `docs/db-schema-and-api-contract.md`.
- API máy đọc: `api/openapi.yaml`.
- Schema thực thi: `db/migrations/*.sql`; query: `db/queries/*.sql`.
- Quy tắc triển khai và kiểm tra: `docs/development.md`.
- `internal/db/` là code sinh tự động: sửa SQL rồi `make sqlc`, không sửa tay.

README chỉ phục vụ quick start cho developer và high-level design; không ghi tiến độ, kết quả test từng đợt hoặc sao chép business rules vào đó. Không sửa README theo từng feature trừ khi cách setup hoặc high-level design thực sự thay đổi. Khi đổi nghiệp vụ/contract, cập nhật nguồn chuẩn tương ứng; tiến độ và kết quả kiểm chứng ghi trong issue, PR, commit hoặc chat.

Chỉ cập nhật tài liệu liên quan thay vì sao chép quy tắc sang index này. Khi hoàn tất: `make lint`, `make test`; thêm `make sqlc` nếu đổi SQL và integration test nếu đổi DB/nghiệp vụ theo hướng dẫn phát triển.
