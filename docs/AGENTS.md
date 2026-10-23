# AGENTS.md — Event Registration Backend

Backend cho hệ thống đăng ký sự kiện của trường (CO3011 · L01 · Nhóm 09).
Luồng cốt lõi: tạo/duyệt sự kiện → đăng ký/hủy → giới hạn chỗ + waitlist → check-in → báo cáo.

## Tech stack

- Go 1.27+, Echo v4 (HTTP), PostgreSQL 16
- `pgx/v5` + `pgxpool` (driver), `sqlc` (sinh code truy vấn), `golang-migrate` (migration)
- `golang-jwt/jwt/v5` (JWT nội bộ) + Google OAuth 2.0 / OpenID Connect (xác minh Google ID token), `excelize` (xuất xlsx)
- Docker Compose cho Postgres local

## Lệnh thường dùng

```bash
make db-up          # docker compose up -d postgres
make migrate-up     # chạy migration trong db/migrations
make migrate-down   # lùi 1 migration
make sqlc           # sinh lại internal/db từ db/queries/*.sql
make seed           # nạp db/seed/seed.sql (đơn vị + email Admin bootstrap @hcmut.edu.vn; không seed mật khẩu)
make run            # chạy cmd/api
make worker         # chạy cmd/worker (outbox email, expire waitlist, complete event)
make test           # unit test (không cần DB)
make test-int       # integration test (cần Postgres đang chạy)
make lint           # golangci-lint
```

Trước khi báo xong một thay đổi: chạy `make sqlc` (nếu sửa query), `make lint`, `make test`; chạy thêm `make test-int` nếu đụng tới module `registration` hoặc `event`.

## Nguồn sự thật

| Nội dung | File | Quy tắc |
| --- | --- | --- |
| API contract | `api/openapi.yaml` | Đổi request/response/path/mã lỗi → sửa file này trước, cùng commit |
| Schema DB | `db/migrations/*.sql` | Không sửa migration đã merge; luôn thêm migration mới, kèm file `.down.sql` |
| Truy vấn | `db/queries/*.sql` | Viết SQL ở đây rồi `make sqlc` |
| Code sinh tự động | `internal/db/` | Không sửa tay |

## Kiến trúc

Clean Architecture áp dụng có chọn lọc. Phụ thuộc chỉ đi một chiều: `adapter → app → domain`.

```
cmd/api, cmd/worker            # composition root: nối adapter vào use case
internal/
  <module>/
    domain/                    # entity, quy tắc, lỗi nghiệp vụ — chỉ import stdlib + shared/kernel
    app/                       # use case + ports.go (interface Repository, UnitOfWork, Notifier, ActivityLogger)
    adapter/http/              # handler, dto, map lỗi domain → HTTP
    adapter/postgres/          # implement port bằng sqlc, map row → entity
  shared/kernel/               # ID, Actor, Clock dùng chung
  platform/                    # hạ tầng: config, pgxpool, JWT, Echo server, middleware
  db/                          # sqlc generated
```

Module và mức độ tách lớp:

- `event`, `registration`: đủ 3 lớp `domain / app / adapter`. Đây là nơi có quy tắc nghiệp vụ. Check-in thuộc module `registration` vì ghi vào cùng bảng.
- `auth`, `unit`, `notification`, `activitylog`, `report`: lược bỏ lớp `domain`, chỉ cần `app` mỏng + adapter. `report` được phép gọi query đọc trực tiếp.

Quy tắc bắt buộc:

- `domain/` và `app/` không import `echo`, `pgx`, `internal/db`, `net/http`.
- Use case trả lỗi domain (`domain.ErrCancelDeadlinePassed`...). Chỉ `adapter/http` mới map sang HTTP status + `error.code`.
- Không tạo interface khi chỉ có một chỗ dùng và không cần thay thế. Interface đặt ở phía dùng (`app/ports.go`), không đặt ở phía implement.
- Lấy thời gian qua `kernel.Clock`, không gọi `time.Now()` trong `domain/` hay `app/`. Quy tắc 24h phải test được.

## Transaction

- Thao tác ghi nhiều bảng chạy trong một `UnitOfWork.Do(ctx, fn)`. Notification và activity log được ghi **trong cùng transaction** với thay đổi trạng thái.
- Mọi thao tác làm đổi số chỗ của một sự kiện (đăng ký, hủy, promote, đổi capacity, hủy sự kiện) phải khóa dòng `events` trước bằng `SELECT … FOR UPDATE`, thông qua `Repository.LockEventForSeatChange`.
- Check-in không khóa `events`. Nó dùng `UPDATE registrations … WHERE status = 'REGISTERED' AND checked_in_at IS NULL`; không có dòng nào được cập nhật nghĩa là đã check-in hoặc không đủ điều kiện, phải đọc lại để phân biệt.
- Gửi email không bao giờ nằm trong transaction hay request. Worker đọc `notifications` có `email_status = 'PENDING'`.

## Bất biến nghiệp vụ — không được phá

Các quy tắc đã chốt với giảng viên. Đổi bất kỳ điều nào cần nhóm đồng ý trước.

1. `registered_count <= capacity` (DB có CHECK `chk_capacity`). Không bao giờ đếm bằng `COUNT(*)` để quyết định còn chỗ.
2. Mỗi sinh viên tối đa 1 đăng ký `REGISTERED`/`WAITLISTED` cho một sự kiện (partial unique index `uq_active_registration`). Map `unique_violation` → `409 ALREADY_REGISTERED`. Đã hủy thì được đăng ký lại, vào cuối waitlist nếu đầy.
3. Có chỗ trống → tự động promote người đầu waitlist theo `queue_seq` (FIFO), trong cùng transaction, không cần ai duyệt. Gửi email lỗi không làm hỏng việc promote. INSERT notification/outbox lỗi thì rollback toàn transaction; không được bỏ qua lỗi ghi DB.
4. Sinh viên tự hủy `REGISTERED` chỉ khi `now < start_at - 24h`. Rời waitlist không giới hạn. Organizer hủy hộ được đến trước `start_at`, bắt buộc lý do, ghi activity log. Không có ngoại lệ cho người được promote muộn hay khi sự kiện đổi giờ/địa điểm trong 24h cuối: những trường hợp này đều hủy qua Organizer.
5. Sự kiện phải qua `PENDING_APPROVAL → PUBLISHED` bởi Admin là APPROVER của đơn vị. Người duyệt ≠ người tạo (`chk_no_self_approve`).
6. Sửa sự kiện đã công bố không cần duyệt lại. Từ 60 phút trước `start_at` (cửa sổ check-in), không cho sửa `start_at`/`end_at` → `409 INVALID_STATE_TRANSITION`; `location` vẫn sửa được. Đổi `start_at`/`end_at`/`location` → notify mọi REGISTERED + WAITLISTED. Tăng capacity → promote. Giảm dưới `registered_count` → `409 CAPACITY_BELOW_REGISTERED`.
7. Hủy sự kiện → mọi REGISTERED/WAITLISTED thành `CANCELLED_BY_EVENT` + notify.
8. Check-in chỉ cho `REGISTERED`, trong cửa sổ từ 60 phút trước `start_at` đến `end_at`. Check-in `MANUAL` bắt buộc `note`. Không có check-out.
9. Phân quyền theo đơn vị qua `unit_memberships`. Organizer quản lý **mọi sự kiện thuộc đơn vị** mà họ có `member_role = 'ORGANIZER'`, không chỉ sự kiện do mình tạo (không kiểm tra `created_by` để phân quyền). Admin xem các đơn vị mà họ là `APPROVER`. Sinh viên chỉ xem đăng ký của chính mình. Truy cập ngoài phạm vi trả `404 NOT_FOUND`, không trả `403`, để không lộ dữ liệu tồn tại.

## Quy ước API

- Base `/api/v1`, JSON `camelCase`, thời gian ISO 8601 có offset; DB lưu `timestamptz` UTC.
- Thành công: `{ "data": ..., "meta": ... }`. Lỗi: `{ "error": { "code", "message", "details" } }`. `code` lấy từ enum `ErrorCode` trong `openapi.yaml`, không tự đặt mã mới mà không thêm vào enum.
- Phân trang `page`/`pageSize` (tối đa 100).
- `PATCH /events/{id}` bắt buộc `If-Match: <version>`.
- Email, MSSV chỉ trả cho Organizer/Admin trong phạm vi; không log dữ liệu cá nhân ra stdout.

## Database

- Tên bảng/cột `snake_case`, khóa chính `uuid` (riêng `activity_logs.id` là `bigint`).
- Không xóa cứng dữ liệu nghiệp vụ; dùng `status` / `is_active`. Ngoại lệ: sự kiện `DRAFT`.
- Mọi log (duyệt, sửa sự kiện, hủy hộ, check-in tay, export) ghi vào `activity_logs`, kèm `event_id` khi liên quan sự kiện; chi tiết đặt trong `metadata`.

## Test

- `domain/`: unit test dạng bảng (table-driven), không DB, dùng clock giả.
- `app/`: test use case với fake repository.
- Integration (`//go:build integration`): chạy với Postgres thật. Bắt buộc giữ các test:
  - 200 request đăng ký song song vào sự kiện 50 chỗ → đúng 50 REGISTERED, 150 WAITLISTED.
  - Hủy 3 đăng ký → 3 người đầu waitlist được promote đúng thứ tự.
  - Hai request check-in cùng QR → 1 thành công, 1 `ALREADY_CHECKED_IN`.
  - Organizer đơn vị A đọc dữ liệu đơn vị B → `404`.

## Không làm

- Không dùng ORM (GORM...). Viết SQL trong `db/queries`.
- Không đọc-rồi-ghi số chỗ mà không khóa dòng `events`.
- Không gọi dịch vụ email/HTTP bên ngoài trong transaction.
- Không thêm dependency mới khi chưa ghi lý do vào mô tả PR.
- Google OAuth 2.0 / OpenID Connect nằm trong phạm vi; không tích hợp SSO khác. Không mở rộng sang check-out, app native, tích hợp điểm rèn luyện, đồng bộ offline, multi-tenant (hệ thống chỉ phục vụ một trường; không thêm `tenant_id`).
- Không thêm API đăng ký tài khoản hay đăng nhập mật khẩu. Google ID token hợp lệ tạo STUDENT ở lần đăng nhập đầu; Admin đầu tiên được bootstrap theo email bằng `db/seed`, phải xác minh Google trước khi sử dụng.

## Google authentication và biên thời gian

- `POST /auth/google` nhận `idToken` từ Google Identity Services qua HTTPS. Backend dùng thư viện xác minh OIDC, kiểm tra chữ ký qua Google JWKS, `iss`, `aud` đúng Google client ID được cấu hình, `exp`, `email_verified = true`, `hd = hcmut.edu.vn` và domain email chính xác `hcmut.edu.vn`. Không tin email/role do client gửi; tham số `hd` ở frontend chỉ là gợi ý UX.
- Định danh Google bằng `sub` duy nhất (`users.google_sub`), không dùng email làm định danh lâu dài. Chỉ liên kết email với user seed chưa có `google_sub` sau khi xác minh đầy đủ; không tự liên kết lại tài khoản đã có sub khác. User bị vô hiệu hóa không được đăng nhập. Tạo/liên kết user trong transaction, chống race bằng unique constraint. Role mặc định STUDENT, không nhận role từ Google/client. Không lưu Google access/refresh token; chỉ cấp JWT và refresh token của hệ thống.
- Khi `now >= start_at`, WAITLISTED hết hiệu lực ngay về nghiệp vụ: API trả EXPIRED, không promote và không tính vào waitlist còn hiệu lực, dù worker chưa cập nhật DB. Worker khóa events trước khi expire và cập nhật waitlist_count về 0 trong cùng transaction; mọi thao tác đổi chỗ phải chuẩn hóa waitlist quá hạn trước khi thực hiện.
- `attendanceRate = 0` khi `registered = 0`; trường hợp khác là `checkedIn / registered`. Sai role trả 403 FORBIDDEN; đúng role nhưng tài nguyên ngoài phạm vi trả 404 NOT_FOUND.
