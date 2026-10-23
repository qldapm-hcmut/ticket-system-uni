# Event Registration System — DB Schema & API Contract

Oct 18, 2026 · @Minh Phan

## 1. Tổng quan & giả định

Hệ thống dùng PostgreSQL và REST/JSON. Mọi bất biến nghiệp vụ đã chốt với thầy (không vượt chỗ, không đăng ký trùng, waitlist FIFO, không check-in trùng) được khóa ở tầng database bằng constraint và transaction, không chỉ dựa vào code ứng dụng.

| Quy tắc đã chốt (nguồn) | Hiện thực trong thiết kế |
| --- | --- |
| Giới hạn chỗ + waitlist, tự chuyển người đầu waitlist (Q3, Q14) | `events.registered_count <= capacity` (CHECK); promote trong cùng transaction với lệnh hủy, chọn dòng WAITLISTED có `queue_seq` nhỏ nhất |
| Gửi email lỗi không ảnh hưởng đăng ký (Q14) | Thay đổi trạng thái và INSERT notification/outbox commit trong cùng transaction; lỗi INSERT rollback toàn bộ. Worker gửi email riêng và retry sau commit |
| Mỗi sinh viên 1 đăng ký còn hiệu lực; được đăng ký lại sau khi hủy (Q15) | Partial unique index `(event_id, student_id) WHERE status IN ('REGISTERED','WAITLISTED')`; đăng ký lại = dòng mới, vào cuối waitlist |
| Tự hủy đến 24h trước giờ bắt đầu (Q13) | Hằng số `CANCEL_CUTOFF = 24h`; quá hạn → `422 CANCEL_DEADLINE_PASSED`; Organizer vẫn hủy hộ được |
| Duyệt trước khi công bố, Organizer không tự duyệt (Q4, Q12) | Máy trạng thái Event; `approve` kiểm tra `approver_id <> created_by`; role ADMIN đảm nhận duyệt |
| Sửa/hủy sau công bố (Q19) | Sửa không cần duyệt lại; đổi thời gian/địa điểm → ghi `activity_logs` (`UPDATE_EVENT`, `isSignificant`) + notify; hủy → hủy toàn bộ registration và waitlist + notify |
| QR + nhập mã + xác nhận tay, không check-out (Q2) | Mỗi registration có `checkin_code` 8 ký tự và QR token ký HMAC; check-in ghi vào cột `checked_in_at` của chính registration, chỉ ghi được khi cột còn NULL |
| Tài khoản trường qua Google OAuth 2.0 / OpenID Connect (phạm vi cập nhật) | Backend xác minh Google ID token, email_verified và domain/hd hcmut.edu.vn; liên kết bằng Google sub, cấp JWT nội bộ |
| Báo cáo + xuất Excel/CSV, không tích hợp ĐRL (Q6) | Endpoint summary + export `csv`/`xlsx` |
| Phân quyền theo đơn vị (Q20) | `unit_memberships` gắn Organizer/Admin với đơn vị; mọi truy vấn quản lý lọc theo đơn vị được giao |

**Giả định của nhóm** (ghi rõ là assumption trong báo cáo, theo Q7):

- Thời gian lưu `timestamptz` (UTC), hiển thị theo Asia/Ho\_Chi\_Minh.
- Quy mô thiết kế: tối đa 50 sự kiện/tháng, 1.000 chỗ/sự kiện, 200 lượt check-in/phút lúc cao điểm.
- Mô hình check-in: thiết bị của BTC quét QR trên điện thoại sinh viên. Sinh viên không cần mạng tại hội trường vì QR đã có sẵn trong trang "Đăng ký của tôi".
- Cửa sổ check-in: từ 60 phút trước `start_at` đến `end_at`.
- Không được giảm `capacity` xuống dưới số đã đăng ký; tăng `capacity` → tự promote từ waitlist.
- Waitlist hết hiệu lực (EXPIRED) ngay khi `now >= start_at`, không phụ thuộc lịch worker. API trả trạng thái hiệu lực EXPIRED cho dòng WAITLISTED quá hạn, không promote và không tính vào waitlist còn hiệu lực (kể cả Event.waitlistCount và báo cáo). Mọi thao tác đổi chỗ chuẩn hóa các dòng quá hạn và bộ đếm trong transaction sau khi khóa events. Worker cũng khóa events, chuyển WAITLISTED → EXPIRED và đặt waitlist_count = 0 trong cùng transaction.
- Phạm vi Organizer (diễn giải "sự kiện do mình quản lý" ở Q20): mọi sự kiện thuộc đơn vị mà Organizer được gán vai trò ORGANIZER, không chỉ sự kiện do chính họ tạo. Lý do: ngày sự kiện cần nhiều người cùng check-in và có người làm thay khi người tạo vắng. Admin chỉ gán Organizer cho người thật sự cần để giới hạn phạm vi xem dữ liệu cá nhân.
- Người được promote từ waitlist khi còn dưới 24h trước sự kiện theo đúng quy tắc 24h như mọi đăng ký khác (Q13 + Q14): không tự hủy được, cần hủy thì liên hệ Organizer hủy hộ.
- Organizer đổi thời gian/địa điểm khi còn dưới 24h (giao giữa Q19 và Q13): sinh viên vẫn nhận notification, quy tắc 24h giữ nguyên; ai muốn hủy thì liên hệ Organizer hủy hộ. Không mở lại quyền tự hủy trong MVP.
- Sửa giờ sự kiện đã công bố được phép (Q19) cho tới trước cửa sổ check-in. Từ 60 phút trước `start_at`, `start_at` và `end_at` bị khóa để không làm sai lượt check-in đã ghi; địa điểm vẫn sửa được. Cần dời sang buổi khác thì hủy sự kiện và tạo sự kiện mới.
- Hệ thống phục vụ một trường (single-tenant); `units` chia dữ liệu bên trong trường. Admin đầu tiên được tạo bằng seed, không có API đăng ký.

Google OAuth 2.0 / OpenID Connect nằm trong phạm vi cập nhật, thay cho đăng nhập mô phỏng; stack backend Go 1.27+, Echo v4, PostgreSQL 16. Ngoài phạm vi: check-out, SSO khác, app native, tích hợp điểm rèn luyện, đồng bộ offline, multi-tenant (hệ thống chỉ phục vụ một trường).

## 2. Mô hình dữ liệu (PostgreSQL)

Schema gồm 8 bảng xoay quanh `events` và `registrations`. Một sinh viên có thể có nhiều dòng `registrations` cho cùng sự kiện (lịch sử hủy/đăng ký lại), nhưng tối đa một dòng còn hiệu lực nhờ partial unique index; check-in là các cột của chính dòng đó.

| Bảng | Vai trò |
| --- | --- |
| `users` | Tài khoản trường liên kết Google; role toàn hệ thống STUDENT / ORGANIZER / ADMIN |
| `units` | Đơn vị tổ chức: Khoa, CLB/Đội/Nhóm, phòng ban |
| `unit_memberships` | Gắn Organizer (tổ chức) và Admin (duyệt) với đơn vị — cơ sở phân quyền Q20 |
| `events` | Sự kiện, sức chứa, bộ đếm chỗ, trạng thái duyệt, lý do từ chối |
| `registrations` | Đăng ký + waitlist (phân biệt bằng `status`) + thông tin check-in |
| `notifications` | Thông báo in-app + outbox gửi email |
| `auth_sessions` | Refresh token nội bộ (chỉ lưu hash), độc lập với Google |
| `activity_logs` | Lịch sử duyệt, lịch sử sửa sự kiện và thao tác nhạy cảm (export, check-in tay, hủy hộ) |

Thiết kế rút gọn từ 11 bảng ban đầu; không chức năng nào bị bỏ:

| Bảng ban đầu | Gộp vào | Lý do | Trade-off chấp nhận |
| --- | --- | --- | --- |
| `check_ins` | Cột `checked_in_*` của `registrations` | Quan hệ 1–0..1: mỗi đăng ký check-in tối đa một lần | Hoàn tác check-in chỉ xóa giá trị cột, không giữ dòng riêng (lịch sử hoàn tác nằm trong `activity_logs`) |
| `event_approval_logs` | `activity_logs` | Cùng cấu trúc: ai, làm gì, lúc nào, trên đối tượng nào | Lý do duyệt/từ chối nằm trong `metadata`; lý do từ chối gần nhất lưu thêm ở `events.reject_reason` |
| `event_change_logs` | `activity_logs` | Chỉ ghi và đọc lại để hiển thị | Danh sách thay đổi cũ/mới nằm trong `metadata` |
| `audit_logs` | `activity_logs` | Dùng chung một cơ chế truy vết | `entity_id` không có FK; bù lại bằng cột `event_id` có FK thật |

### 2.1 ERD

&#91;embedded content: ERD · 8 bảng, ký hiệu crow's foot\]

USERS và UNITS quan hệ nhiều-nhiều qua UNIT\_MEMBERSHIPS; USERS còn nối EVENTS hai lần (người tạo, người duyệt), hình chỉ vẽ quan hệ người tạo để đỡ rối.

### 2.2 Mapping ERD → schema

Mỗi thực thể thành một bảng `snake_case` số nhiều với khóa `uuid`; quan hệ 1–N thành FK ở phía N, N–N thành bảng trung gian, 1–0..1 gộp thành cột nullable của bảng phía 1. Không xóa cứng dữ liệu nghiệp vụ (dùng `status`, `is_active`), nên mọi FK để mặc định `ON DELETE NO ACTION`.

| Quan hệ trong ERD | Bản số | Hiện thực trong schema |
| --- | --- | --- |
| USERS — UNITS qua UNIT\_MEMBERSHIPS | N–N | Bảng trung gian, PK `(user_id, unit_id, member_role)`; cùng một người có thể vừa ORGANIZER vừa APPROVER |
| UNITS — EVENTS | 1–N | `events.unit_id` NOT NULL → `units.id` |
| USERS — EVENTS (tạo) | 1–N | `events.created_by` NOT NULL → `users.id` |
| USERS — EVENTS (duyệt) | 1–0..N | `events.approved_by` NULL → `users.id`; `chk_no_self_approve` |
| EVENTS — REGISTRATIONS | 1–N | `registrations.event_id`; partial UNIQUE `(event_id, student_id)` khi còn hiệu lực |
| USERS — REGISTRATIONS (đăng ký) | 1–N | `registrations.student_id` NOT NULL → `users.id` |
| USERS — REGISTRATIONS (người check-in) | 1–0..N | `registrations.checked_in_by` NULL → `users.id` |
| EVENTS — ACTIVITY\_LOGS | 1–0..N | `activity_logs.event_id` NULL → `events.id`; NULL với log không gắn sự kiện (đổi role, gán đơn vị) |
| USERS — ACTIVITY\_LOGS | 1–0..N | `activity_logs.actor_id` NULL → `users.id` (job hệ thống không có actor) |
| Đối tượng bất kỳ — ACTIVITY\_LOGS | 1–0..N | `entity_type` + `entity_id`, không FK (quan hệ đa hình) |
| USERS — NOTIFICATIONS, EVENTS — NOTIFICATIONS | 1–N, 1–0..N | `user_id` NOT NULL, `event_id` NULL |
| USERS — AUTH\_SESSIONS | 1–N | `auth_sessions.user_id` NOT NULL |

| Thuộc tính đặc biệt | Cách map |
| --- | --- |
| Check-in (thực thể yếu 1–0..1 của đăng ký) | 4 cột nullable `checked_in_at`, `checkin_method`, `checked_in_by`, `checkin_note`; CHECK buộc 3 cột đầu cùng NULL hoặc cùng có giá trị |
| Thuộc tính dẫn xuất `registered_count`, `waitlist_count` | Lưu thành cột bộ đếm, cập nhật trong transaction, để CHECK chống vượt chỗ |
| `waitlistPosition` | Không lưu; tính khi đọc từ `queue_seq` |
| Chi tiết từng loại log (lý do, thay đổi cũ/mới, cờ quan trọng) | Một cột `metadata jsonb` thay cho các cột riêng của 3 bảng log cũ |
| Miền giá trị cố định (role, status, method…) | PostgreSQL `ENUM`; `activity_logs.action` để `varchar` vì danh sách action còn mở rộng |

### 2.3 DDL PostgreSQL

```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS citext;

-- ========== ENUMS ==========
CREATE TYPE user_role           AS ENUM ('STUDENT','ORGANIZER','ADMIN');
CREATE TYPE unit_type           AS ENUM ('FACULTY','CLUB','DEPARTMENT');
CREATE TYPE membership_role     AS ENUM ('ORGANIZER','APPROVER');
CREATE TYPE event_status        AS ENUM ('DRAFT','PENDING_APPROVAL','REJECTED',
                                         'PUBLISHED','CANCELLED','COMPLETED');
CREATE TYPE registration_status AS ENUM ('REGISTERED','WAITLISTED','CANCELLED',
                                         'CANCELLED_BY_EVENT','EXPIRED');
CREATE TYPE cancel_actor        AS ENUM ('STUDENT','ORGANIZER','SYSTEM');
CREATE TYPE checkin_method      AS ENUM ('QR','CODE','MANUAL');
CREATE TYPE notification_type   AS ENUM ('REGISTERED','WAITLISTED','PROMOTED','REG_CANCELLED',
                                         'EVENT_CHANGED','EVENT_CANCELLED',
                                         'EVENT_APPROVED','EVENT_REJECTED','REMINDER');
CREATE TYPE delivery_status     AS ENUM ('PENDING','SENT','FAILED');

-- ========== 1. USERS ==========
CREATE TABLE users (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email         citext NOT NULL UNIQUE CHECK (email LIKE '%@hcmut.edu.vn'),
  full_name     varchar(120) NOT NULL,
  student_code  varchar(10) UNIQUE,              -- MSSV; NULL với cán bộ
  role          user_role NOT NULL DEFAULT 'STUDENT',
  google_sub    text UNIQUE,                     -- Google sub; NULL chỉ với tài khoản bootstrap chưa liên kết
  is_active     boolean NOT NULL DEFAULT true,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

-- ========== 2. UNITS ==========
CREATE TABLE units (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code       varchar(30) NOT NULL UNIQUE,        -- vd: CSE, CLB-GDSC, P.CTSV
  name       varchar(200) NOT NULL,
  type       unit_type NOT NULL,
  is_active  boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- ========== 3. UNIT_MEMBERSHIPS ==========
CREATE TABLE unit_memberships (
  user_id     uuid NOT NULL REFERENCES users(id),
  unit_id     uuid NOT NULL REFERENCES units(id),
  member_role membership_role NOT NULL,          -- ORGANIZER: tạo/quản lý; APPROVER: duyệt (user.role = ADMIN)
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, unit_id, member_role)
);
CREATE INDEX idx_membership_unit ON unit_memberships (unit_id, member_role);

-- ========== 4. EVENTS ==========
CREATE TABLE events (
  id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  unit_id                uuid NOT NULL REFERENCES units(id),
  created_by             uuid NOT NULL REFERENCES users(id),
  title                  varchar(200) NOT NULL,
  description            text,
  location               varchar(255) NOT NULL,
  start_at               timestamptz NOT NULL,
  end_at                 timestamptz NOT NULL,
  registration_opens_at  timestamptz NOT NULL,
  registration_closes_at timestamptz NOT NULL,
  capacity               integer NOT NULL CHECK (capacity > 0),
  waitlist_limit         integer CHECK (waitlist_limit >= 0),  -- NULL = không giới hạn
  registered_count       integer NOT NULL DEFAULT 0,          -- bộ đếm, cập nhật trong transaction
  waitlist_count         integer NOT NULL DEFAULT 0,
  status                 event_status NOT NULL DEFAULT 'DRAFT',
  approved_by            uuid REFERENCES users(id),
  approved_at            timestamptz,
  reject_reason          text,                                -- lý do từ chối gần nhất, hiển thị cho Organizer
  published_at           timestamptz,
  cancelled_at           timestamptz,
  cancel_reason          text,
  version                integer NOT NULL DEFAULT 1,          -- optimistic lock cho PATCH
  created_at             timestamptz NOT NULL DEFAULT now(),
  updated_at             timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT chk_event_time      CHECK (end_at > start_at),
  CONSTRAINT chk_reg_window      CHECK (registration_opens_at < registration_closes_at
                                        AND registration_closes_at <= start_at),
  CONSTRAINT chk_capacity        CHECK (registered_count BETWEEN 0 AND capacity),   -- chống overbooking
  CONSTRAINT chk_waitlist        CHECK (waitlist_count >= 0 AND
                                        (waitlist_limit IS NULL OR waitlist_count <= waitlist_limit)),
  CONSTRAINT chk_no_self_approve CHECK (approved_by IS NULL OR approved_by <> created_by)  -- Q12
);
CREATE INDEX idx_events_catalog     ON events (start_at) WHERE status = 'PUBLISHED';
CREATE INDEX idx_events_unit_status ON events (unit_id, status, start_at);

-- ========== 5. REGISTRATIONS (đăng ký + waitlist + check-in) ==========
CREATE TABLE registrations (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id          uuid NOT NULL REFERENCES events(id),
  student_id        uuid NOT NULL REFERENCES users(id),
  status            registration_status NOT NULL,
  queue_seq         bigint GENERATED ALWAYS AS IDENTITY,  -- thứ tự vào hàng, dùng cho FIFO
  promoted_at       timestamptz,                         -- != NULL nếu đi lên từ waitlist
  cancelled_at      timestamptz,
  cancelled_by      cancel_actor,
  cancelled_by_user uuid REFERENCES users(id),
  cancel_reason     varchar(255),
  checkin_code      char(8) NOT NULL,                    -- mã nhập tay, Crockford base32
  -- check-in (gộp từ bảng check_ins cũ)
  checked_in_at     timestamptz,
  checkin_method    checkin_method,
  checked_in_by     uuid REFERENCES users(id),
  checkin_note      varchar(255),                        -- bắt buộc khi MANUAL
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT uq_checkin_code UNIQUE (event_id, checkin_code),
  CONSTRAINT chk_cancel_fields CHECK (
    (status IN ('CANCELLED','CANCELLED_BY_EVENT')) = (cancelled_at IS NOT NULL)),
  CONSTRAINT chk_checkin_fields CHECK (
    (checked_in_at IS NULL) = (checkin_method IS NULL) AND
    (checked_in_at IS NULL) = (checked_in_by  IS NULL)),
  CONSTRAINT chk_manual_note CHECK (
    checkin_method IS DISTINCT FROM 'MANUAL' OR checkin_note IS NOT NULL)
);
-- Q15: 1 đăng ký còn hiệu lực / sinh viên / sự kiện; dòng đã hủy không chặn đăng ký lại
CREATE UNIQUE INDEX uq_active_registration ON registrations (event_id, student_id)
  WHERE status IN ('REGISTERED','WAITLISTED');
-- Lấy người đầu waitlist: O(log n)
CREATE INDEX idx_waitlist_head  ON registrations (event_id, queue_seq) WHERE status = 'WAITLISTED';
CREATE INDEX idx_reg_event      ON registrations (event_id, status);
CREATE INDEX idx_reg_student    ON registrations (student_id, created_at DESC);
-- Báo cáo điểm danh
CREATE INDEX idx_reg_checked_in ON registrations (event_id, checked_in_at)
  WHERE checked_in_at IS NOT NULL;

-- ========== 6. NOTIFICATIONS (in-app + outbox email) ==========
CREATE TABLE notifications (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id        uuid NOT NULL REFERENCES users(id),
  event_id       uuid REFERENCES events(id),
  type           notification_type NOT NULL,
  title          varchar(200) NOT NULL,
  body           text NOT NULL,
  read_at        timestamptz,
  email_status   delivery_status NOT NULL DEFAULT 'PENDING',
  email_attempts smallint NOT NULL DEFAULT 0,
  last_error     text,
  created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notif_user   ON notifications (user_id, created_at DESC);
CREATE INDEX idx_notif_outbox ON notifications (created_at) WHERE email_status = 'PENDING';

-- ========== 7. AUTH_SESSIONS ==========
CREATE TABLE auth_sessions (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id            uuid NOT NULL REFERENCES users(id),
  refresh_token_hash text NOT NULL UNIQUE,
  expires_at         timestamptz NOT NULL,
  revoked_at         timestamptz,
  created_at         timestamptz NOT NULL DEFAULT now()
);

-- ========== 8. ACTIVITY_LOGS (gộp approval/change/audit logs) ==========
CREATE TABLE activity_logs (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_id    uuid REFERENCES users(id),           -- NULL khi job hệ thống
  event_id    uuid REFERENCES events(id),          -- FK thật cho log liên quan sự kiện
  entity_type varchar(30) NOT NULL,                -- EVENT, REGISTRATION, USER, UNIT
  entity_id   uuid,
  action      varchar(40) NOT NULL,                -- xem bảng action bên dưới
  metadata    jsonb,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_activity_event  ON activity_logs (event_id, created_at);
CREATE INDEX idx_activity_entity ON activity_logs (entity_type, entity_id);
```

Các action ghi vào `activity_logs`. Lịch sử duyệt (P5) là truy vấn `WHERE event_id = :id AND action IN ('SUBMIT','APPROVE','REJECT')`.

| `action` | `entity_type` | Có `event_id` | `metadata` |
| --- | --- | --- | --- |
| `SUBMIT`, `APPROVE`, `REJECT` | EVENT | Có | `{ "comment" }` hoặc `{ "reason" }` |
| `UPDATE_EVENT` | EVENT | Có | `{ "changes": { "startAt": { "old", "new" } }, "isSignificant": true }` |
| `CANCEL_EVENT` | EVENT | Có | `{ "reason", "affectedCount" }` |
| `CANCEL_ON_BEHALF` | REGISTRATION | Có | `{ "reason", "studentId" }` |
| `MANUAL_CHECKIN`, `UNDO_CHECKIN` | REGISTRATION | Có | `{ "note", "previousCheckedInAt" }` |
| `EXPORT_DATA` | EVENT | Có | `{ "kind", "format", "rowCount" }` |
| `UPDATE_USER`, `SET_UNIT_MEMBERS` | USER, UNIT | Không | `{ "before", "after" }` |

Notification được INSERT trong cùng transaction với thay đổi trạng thái, nên bản in-app luôn tồn tại; chỉ việc gửi email là bất đồng bộ và có thể FAILED mà không ảnh hưởng đăng ký (Q14).

## 3. Máy trạng thái

&#91;embedded content: máy trạng thái · Event 6 trạng thái, Registration 5 trạng thái + check-in\]

Chỉ PUBLISHED xuất hiện ở danh mục E1 và nhận đăng ký; REGISTERED là trạng thái duy nhất được check-in. Check-in được ghi bằng các cột checked\_in\_\* chứ không phải một status, nên hoàn tác check-in (C3) không làm đổi trạng thái đăng ký. Mọi chuyển trạng thái không có mũi tên trên sơ đồ đều trả `409 INVALID_STATE_TRANSITION`.

## 4. Quy ước API

REST/JSON, base URL `/api/v1`, xác thực bằng JWT Bearer. Mọi lỗi nghiệp vụ trả về một mã `code` ổn định để frontend xử lý, không dựa vào câu chữ `message`.

- **Định dạng:** JSON, field `camelCase`; thời gian ISO 8601 có offset (`2026-11-05T08:00:00+07:00`), server lưu UTC.
- **Xác thực:** `Authorization: Bearer <accessToken>`; access token 15 phút, refresh token 7 ngày (lưu hash trong `auth_sessions`).
- **Phân trang:** `?page=1&pageSize=20` (tối đa 100); response có `meta: { page, pageSize, totalItems, totalPages }`.
- **Optimistic lock:** `PATCH /events/{id}` bắt buộc header `If-Match: <version>`; sai version → `409 VERSION_CONFLICT`.
- **Idempotency:** đăng ký và check-in tự idempotent nhờ unique constraint — gọi lại trả về bản ghi đã có kèm mã lỗi tương ứng, không tạo bản ghi mới.

Response thành công:

```json
{ "data": { ... }, "meta": { ... } }
```

Response lỗi:

```json
{
  "error": {
    "code": "CANCEL_DEADLINE_PASSED",
    "message": "Chỉ được hủy trước giờ bắt đầu 24 giờ.",
    "details": { "deadline": "2026-11-04T08:00:00+07:00" }
  }
}
```

| HTTP | `code` | Khi nào |
| --- | --- | --- |
| 400 | `VALIDATION_ERROR` | Body/query sai kiểu hoặc thiếu field; `details.fields[]` |
| 401 | `UNAUTHENTICATED` | Thiếu/hết hạn token |
| 403 | `FORBIDDEN` | Sai role cho thao tác; tài nguyên ngoài đơn vị/phạm vi trả 404 |
| 403 | `SELF_APPROVAL_NOT_ALLOWED` | Người duyệt là người tạo sự kiện |
| 404 | `NOT_FOUND` | Không tồn tại, hoặc tồn tại nhưng người gọi không được thấy (tránh lộ dữ liệu) |
| 409 | `ALREADY_REGISTERED` | Đã có đăng ký REGISTERED/WAITLISTED cho sự kiện này |
| 409 | `EVENT_FULL` | Hết chỗ và waitlist đã đạt `waitlistLimit` |
| 409 | `INVALID_STATE_TRANSITION` | Thao tác không hợp lệ với trạng thái hiện tại |
| 409 | `CAPACITY_BELOW_REGISTERED` | Giảm sức chứa xuống dưới số đã đăng ký |
| 409 | `VERSION_CONFLICT` | `If-Match` không khớp `version` |
| 409 | `ALREADY_CHECKED_IN` | Check-in trùng; `details.checkedInAt` |
| 422 | `REGISTRATION_CLOSED` | Ngoài cửa sổ đăng ký hoặc sự kiện không ở PUBLISHED |
| 422 | `CANCEL_DEADLINE_PASSED` | Sinh viên tự hủy khi còn < 24h |
| 422 | `CHECKIN_WINDOW_CLOSED` | Ngoài cửa sổ check-in |
| 422 | `NOT_ELIGIBLE_FOR_CHECKIN` | Registration không ở trạng thái REGISTERED |
| 422 | `INVALID_QR` | Chữ ký QR sai hoặc QR thuộc sự kiện khác |

## 5. API contract

33 endpoint chia 8 nhóm, bám theo workflow nghiệm thu: tạo/duyệt sự kiện → đăng ký/hủy → waitlist → check-in → báo cáo. Cột Role là role tối thiểu; quyền theo đơn vị xem mục 6.

| # | Method | Path | Role | Mô tả |
| --- | --- | --- | --- | --- |
| A1 | POST | `/auth/google` | Public | Xác minh Google ID token của tài khoản trường, cấp token nội bộ |
| A2 | POST | `/auth/refresh` | Public | Đổi refresh token lấy access token mới |
| A3 | POST | `/auth/logout` | Đã đăng nhập | Thu hồi refresh token |
| A4 | GET | `/me` | Đã đăng nhập | Thông tin user + danh sách đơn vị và vai trò |
| U1 | GET | `/units` | Đã đăng nhập | Danh sách đơn vị (lọc sự kiện) |
| U2 | POST | `/admin/units` | Admin | Tạo đơn vị |
| U3 | PUT | `/admin/units/{unitId}/members` | Admin | Gán Organizer/Approver cho đơn vị |
| U4 | GET | `/admin/users` | Admin | Tìm user, đổi role qua U5 |
| U5 | PATCH | `/admin/users/{userId}` | Admin | Đổi `role`, `isActive` |
| E1 | GET | `/events` | Đã đăng nhập | Danh mục sự kiện PUBLISHED (cho sinh viên) |
| E2 | GET | `/events/{eventId}` | Đã đăng nhập | Chi tiết; kèm `myRegistration` nếu là sinh viên |
| E3 | GET | `/manage/events` | Organizer, Admin | Sự kiện mọi trạng thái trong đơn vị được giao |
| E4 | POST | `/events` | Organizer | Tạo sự kiện (DRAFT) |
| E5 | PATCH | `/events/{eventId}` | Organizer | Sửa; cần `If-Match` |
| E6 | DELETE | `/events/{eventId}` | Organizer | Xóa, chỉ khi DRAFT |
| E7 | POST | `/events/{eventId}/cancel` | Organizer | Hủy sự kiện đã công bố |
| P1 | POST | `/events/{eventId}/submit` | Organizer | Gửi duyệt (DRAFT/REJECTED → PENDING\_APPROVAL) |
| P2 | GET | `/approvals/pending` | Admin | Hàng chờ duyệt trong đơn vị được giao |
| P3 | POST | `/events/{eventId}/approve` | Admin | Duyệt và công bố |
| P4 | POST | `/events/{eventId}/reject` | Admin | Từ chối, bắt buộc lý do |
| P5 | GET | `/events/{eventId}/approval-logs` | Organizer, Admin | Lịch sử duyệt |
| R1 | POST | `/events/{eventId}/registrations` | Student | Đăng ký; tự vào waitlist nếu hết chỗ |
| R2 | POST | `/registrations/{registrationId}/cancel` | Student, Organizer | Hủy; tự promote người đầu waitlist |
| R3 | GET | `/me/registrations` | Student | Đăng ký của tôi (lọc theo status, thời gian) |
| R4 | GET | `/registrations/{registrationId}` | Student (chủ sở hữu), Organizer | Chi tiết + QR token + mã nhập tay |
| R5 | GET | `/events/{eventId}/registrations` | Organizer, Admin | Danh sách đăng ký / waitlist theo thứ tự |
| C1 | POST | `/events/{eventId}/check-ins` | Organizer | Check-in bằng QR, mã, hoặc xác nhận tay |
| C2 | GET | `/events/{eventId}/check-ins` | Organizer, Admin | Danh sách đã check-in |
| C3 | DELETE | `/registrations/{registrationId}/check-in` | Organizer | Hoàn tác check-in nhầm (ghi audit) |
| B1 | GET | `/events/{eventId}/reports/summary` | Organizer, Admin | Thống kê số lượng |
| B2 | GET | `/events/{eventId}/exports/{kind}` | Organizer, Admin | Xuất `registrations` hoặc `attendance`, `?format=csv\|xlsx` |
| N1 | GET | `/me/notifications` | Đã đăng nhập | Thông báo in-app, `?unread=true` |
| N2 | POST | `/me/notifications/read` | Đã đăng nhập | Đánh dấu đã đọc: `{ "ids": [...] }` hoặc `{ "all": true }` |

### 5.1 Auth

**A1 `POST /auth/google`**

```json
// Request
{ "idToken": "<Google ID token từ Google Identity Services>" }

// 200
{
  "data": {
    "accessToken": "eyJhbGciOi...",
    "refreshToken": "rt_9f2c...",
    "expiresIn": 900,
    "user": {
      "id": "6c1e...", "email": "an.nguyen@hcmut.edu.vn", "fullName": "Nguyễn Văn An",
      "studentCode": "2210001", "role": "STUDENT", "units": []
    }
  }
}
```

Frontend lấy ID token qua Google Identity Services; gửi qua HTTPS tới backend. Backend dùng thư viện OIDC xác minh chữ ký với Google JWKS, issuer Google hợp lệ, `aud` khớp Google client ID cấu hình, thời hạn `exp`, `email_verified = true`, `hd = hcmut.edu.vn` và domain email chính xác `hcmut.edu.vn`. Không chỉ kiểm tra hậu tố chuỗi hoặc tin tham số `hd` của frontend. Không nhận email/role tùy ý từ client.

Định danh bằng `google_sub` (`sub` của Google). Lần đầu tạo STUDENT; user seed chưa có sub được liên kết theo email đã xác minh trong transaction. Không liên kết nếu email đã gắn sub khác; unique email/sub bảo vệ đăng nhập song song. Không tự nâng role. Admin đầu tiên seed email thật, không có mật khẩu. Không lưu token Google; refresh/logout dùng session nội bộ, không đăng xuất Google. Không log ID token, access token hay refresh token.

Lỗi: `401 UNAUTHENTICATED` (token sai/hết hạn, sai audience/domain/hd, email chưa xác minh, xung đột liên kết hoặc `isActive = false`). JWT payload: `sub`, `role`, `exp`; đơn vị được giao không nhét vào token mà tra DB mỗi request để đổi quyền có hiệu lực ngay.

### 5.2 Events

**Event object** (dùng chung E1–E7):

```json
{
  "id": "e7a1...",
  "unit": { "id": "u01...", "code": "CSE", "name": "Khoa KH&KT Máy tính" },
  "title": "Workshop: System Design 101",
  "description": "...",
  "location": "H6-112",
  "startAt": "2026-11-05T08:00:00+07:00",
  "endAt": "2026-11-05T11:00:00+07:00",
  "registrationOpensAt": "2026-10-25T00:00:00+07:00",
  "registrationClosesAt": "2026-11-04T23:59:00+07:00",
  "capacity": 120,
  "waitlistLimit": 50,
  "registeredCount": 120,
  "waitlistCount": 7,
  "availableSeats": 0,
  "isRegistrationOpen": true,
  "cancelDeadline": "2026-11-04T08:00:00+07:00",
  "status": "PUBLISHED",
  "version": 3,
  "createdBy": { "id": "...", "fullName": "..." },
  "myRegistration": { "id": "r55...", "status": "WAITLISTED", "waitlistPosition": 3 }
}
```

`myRegistration` chỉ trả cho sinh viên (null nếu chưa đăng ký); `createdBy` và `version` chỉ trả cho Organizer/Admin.

**E1 `GET /events`** — query: `q`, `unitId`, `from`, `to`, `availableOnly=true`, `page`, `pageSize`. Chỉ trả `status = PUBLISHED`, sắp theo `startAt` tăng dần.

**E3 `GET /manage/events`** — query thêm `status` (nhiều giá trị, phân cách dấu phẩy). Organizer thấy sự kiện của đơn vị mình là ORGANIZER; Admin thấy sự kiện của đơn vị mình là APPROVER.

**E4 `POST /events`**

```json
// Request
{
  "unitId": "u01...",
  "title": "Workshop: System Design 101",
  "description": "...",
  "location": "H6-112",
  "startAt": "2026-11-05T08:00:00+07:00",
  "endAt": "2026-11-05T11:00:00+07:00",
  "registrationOpensAt": "2026-10-25T00:00:00+07:00",
  "registrationClosesAt": "2026-11-04T23:59:00+07:00",
  "capacity": 120,
  "waitlistLimit": 50
}
// 201 → Event object, status = DRAFT
```

Validation: người gọi phải là ORGANIZER của `unitId`; `endAt > startAt`; `registrationClosesAt <= startAt`; `startAt` ở tương lai; `capacity` 1–5000.

**E5 `PATCH /events/{eventId}`** — header `If-Match: 3`, body là tập con field của E4 (trừ `unitId`).

- DRAFT / REJECTED: sửa mọi field.
- PENDING\_APPROVAL: không cho sửa → `409 INVALID_STATE_TRANSITION` (muốn sửa thì Admin reject trước).
- PUBLISHED: sửa được, không cần duyệt lại (Q19). Từ 60 phút trước startAt không sửa được startAt/endAt → 409 INVALID\_STATE\_TRANSITION; location vẫn sửa được. Nếu đổi `startAt`, `endAt` hoặc `location` → ghi `activity_logs (UPDATE_EVENT, isSignificant: true)` và tạo notification `EVENT_CHANGED` cho mọi REGISTERED + WAITLISTED. Tăng `capacity` → promote tự động; giảm dưới `registeredCount` → `409 CAPACITY_BELOW_REGISTERED`.
- CANCELLED / COMPLETED: không sửa được.

```json
// 200
{ "data": { ...Event object, "version": 4 },
  "meta": { "significantChange": true, "notifiedCount": 127, "promotedCount": 0 } }
```

**E7 `POST /events/{eventId}/cancel`** — `{ "reason": "Diễn giả bận đột xuất" }`. PUBLISHED → CANCELLED; mọi registration REGISTERED/WAITLISTED → `CANCELLED_BY_EVENT`, đếm về 0, notify `EVENT_CANCELLED`. Trả `meta.affectedCount`.

### 5.3 Duyệt sự kiện

**P1 `POST /events/{eventId}/submit`** — không body. DRAFT/REJECTED → PENDING\_APPROVAL, ghi log `SUBMIT`.

**P3 `POST /events/{eventId}/approve`** — `{ "comment": "OK" }` (tùy chọn). Kiểm tra: user.role = ADMIN, là APPROVER của đơn vị sự kiện, và `userId <> createdBy` Sai role → `403 FORBIDDEN`; ngoài đơn vị duyệt → `404 NOT_FOUND`; người duyệt trùng người tạo → `403 SELF_APPROVAL_NOT_ALLOWED` (chỉ kiểm tra sau quyền truy cập). Nếu `startAt` đã qua → `422 VALIDATION_ERROR`. Thành công: status = PUBLISHED, set `approvedBy`, `approvedAt`, `publishedAt`, notify Organizer `EVENT_APPROVED`.

**P4 `POST /events/{eventId}/reject`** — `{ "reason": "Thiếu thông tin địa điểm" }` (bắt buộc). PENDING\_APPROVAL → REJECTED, lưu lý do vào events.reject\_reason và activity\_logs, notify Organizer `EVENT_REJECTED`.

### 5.4 Đăng ký & waitlist

**R1 `POST /events/{eventId}/registrations`** — không body; sinh viên lấy từ token.

```json
// 201 — còn chỗ
{ "data": { "id": "r55...", "eventId": "e7a1...", "status": "REGISTERED",
            "waitlistPosition": null, "checkinCode": "K7Q2-M9XD", "createdAt": "..." } }

// 201 — hết chỗ, vào waitlist
{ "data": { "id": "r56...", "eventId": "e7a1...", "status": "WAITLISTED",
            "waitlistPosition": 8, "checkinCode": null, "createdAt": "..." } }
```

Lỗi: `409 ALREADY_REGISTERED` (kèm `details.registrationId`), `409 EVENT_FULL`, `422 REGISTRATION_CLOSED`. `checkinCode` chỉ hiện khi REGISTERED.

**R2 `POST /registrations/{registrationId}/cancel`** — `{ "reason": "..." }` (tùy chọn với sinh viên, bắt buộc khi Organizer hủy hộ).

- Sinh viên chủ sở hữu: chỉ khi `now < startAt − 24h`, ngược lại `422 CANCEL_DEADLINE_PASSED` (Q13). Rời waitlist cũng dùng endpoint này và không bị chặn bởi hạn 24h.
- Organizer của đơn vị: hủy hộ bất kỳ lúc nào trước `startAt`, ghi `activity_logs`.

```json
// 200
{ "data": { "id": "r55...", "status": "CANCELLED", "cancelledAt": "...", "cancelledBy": "STUDENT" },
  "meta": { "promotedRegistrationId": "r40..." } }
```

**R3 `GET /me/registrations`** — query `status`, `upcoming=true`. Mỗi phần tử gồm registration + event rút gọn (`id`, `title`, `startAt`, `location`, `status`).

**R4 `GET /registrations/{registrationId}`** — trả thêm `qrToken` khi REGISTERED:

```json
{ "data": { "id": "r55...", "status": "REGISTERED", "checkinCode": "K7Q2-M9XD",
            "qrToken": "v1.r55....e7a1....3f9c0b2a1d",
            "checkIn": null, "event": { ... } } }
```

`qrToken = v1.<registrationId>.<eventId>.<HMAC-SHA256(secret, registrationId|eventId)>`, frontend tự vẽ QR từ chuỗi này. Token không chứa thông tin cá nhân và không tự hết hạn; hiệu lực được kiểm tra bằng trạng thái registration lúc quét.

**R5 `GET /events/{eventId}/registrations`** — query `status=REGISTERED|WAITLISTED|CANCELLED`, `q` (tên/MSSV), `page`. Waitlist sắp theo `queueSeq` tăng dần và trả `waitlistPosition`. Mỗi dòng: `student { id, fullName, studentCode, email }`, `status`, `createdAt`, `promotedAt`, `checkedInAt`.

### 5.5 Check-in

**C1 `POST /events/{eventId}/check-ins`** — một trong ba dạng:

```json
{ "method": "QR",     "qrToken": "v1.r55....e7a1....3f9c0b2a1d" }
{ "method": "CODE",   "checkinCode": "K7Q2M9XD" }
{ "method": "MANUAL", "registrationId": "r55...", "note": "SV quên điện thoại, đã đối chiếu thẻ SV" }
```

```json
// 201
{ "data": { "registrationId": "r55...", "method": "QR",
            "checkedInAt": "2026-11-05T07:52:10+07:00",
            "checkedInBy": { "id": "...", "fullName": "Trần Thị B" },
            "student": { "fullName": "Nguyễn Văn An", "studentCode": "2210001" } } }
```

**C3 `DELETE /registrations/{registrationId}/check-in`** — hoàn tác check-in nhầm: đưa 4 cột `checked_in_*` về NULL và ghi `activity_logs` (`UNDO_CHECKIN`, kèm thời điểm check-in cũ). Trả `204`; chưa check-in → `404 NOT_FOUND`.

Lỗi: `409 ALREADY_CHECKED_IN` (kèm `details.checkedInAt`, `details.checkedInBy` để BTC xử lý tại chỗ), `422 INVALID_QR`, `422 NOT_ELIGIBLE_FOR_CHECKIN`, `422 CHECKIN_WINDOW_CLOSED`. Mã nhập tay không phân biệt hoa thường và bỏ qua dấu `-`. Check-in MANUAL bắt buộc `note` và ghi `activity_logs`.

### 5.6 Báo cáo & xuất file

**B1 `GET /events/{eventId}/reports/summary`**

```json
{ "data": {
    "capacity": 120, "registered": 118, "waitlisted": 0, "cancelled": 14,
    "promotedFromWaitlist": 9, "checkedIn": 101, "attendanceRate": 0.856,
    "checkInByMethod": { "QR": 92, "CODE": 6, "MANUAL": 3 },
    "generatedAt": "2026-11-05T11:05:00+07:00" } }
```

`attendanceRate = registered == 0 ? 0 : checkedIn / registered`. `waitlisted` chỉ đếm WAITLISTED còn hiệu lực (`now < start_at`), trả 0 từ start_at dù worker chưa chạy.

**B2 `GET /events/{eventId}/exports/{kind}?format=xlsx`** — `kind = registrations | attendance`. Trả file trực tiếp (`Content-Disposition: attachment`), CSV có BOM UTF-8 để Excel đọc đúng tiếng Việt. Cột `attendance`: STT, MSSV, Họ tên, Email, Trạng thái đăng ký, Thời gian check-in, Phương thức. Mỗi lần xuất ghi `activity_logs` (dữ liệu cá nhân).

### 5.7 Notifications

**N1 `GET /me/notifications`**

```json
{ "data": [ { "id": "n01...", "type": "PROMOTED", "eventId": "e7a1...",
              "title": "Bạn đã có chỗ tại Workshop: System Design 101",
              "body": "Bạn được chuyển từ danh sách chờ sang danh sách đăng ký.",
              "readAt": null, "createdAt": "..." } ],
  "meta": { "unreadCount": 2, "page": 1, "pageSize": 20, "totalItems": 5, "totalPages": 1 } }
```

Nhắc lịch (`REMINDER`) do job chạy 24h trước `startAt` — chức năng bổ trợ, làm sau cùng (Q1).

## 6. Phân quyền & xử lý đồng thời

Quyền được xét hai lớp: role toàn hệ thống (`users.role`) rồi đến đơn vị (`unit_memberships`). Mọi thao tác làm thay đổi số chỗ của một sự kiện đều khóa dòng `events` đó (`SELECT … FOR UPDATE`), nên các request cùng sự kiện chạy tuần tự còn các sự kiện khác nhau vẫn song song.

### 6.1 Ma trận phân quyền

| Thao tác | Student | Organizer (đơn vị mình) | Admin (đơn vị được giao duyệt) |
| --- | --- | --- | --- |
| Xem danh mục sự kiện PUBLISHED | Có | Có | Có |
| Tạo / sửa / xóa nháp / hủy sự kiện | — | Có | — |
| Gửi duyệt | — | Có | — |
| Duyệt / từ chối | — | — | Có, trừ sự kiện do chính mình tạo |
| Đăng ký, tự hủy, rời waitlist | Có (của mình) | — | — |
| Hủy đăng ký hộ sinh viên | — | Có | — |
| Xem danh sách đăng ký, waitlist, điểm danh | Chỉ đăng ký của mình | Có | Có (chỉ đọc) |
| Check-in, hoàn tác check-in | — | Có | — |
| Xem báo cáo, xuất Excel/CSV | — | Có | Có |
| Quản lý user, đơn vị, gán thành viên | — | — | Có |

Truy cập tài nguyên ngoài phạm vi trả `404 NOT_FOUND` thay vì `403`, để không lộ việc sự kiện hay đăng ký đó tồn tại. Dữ liệu cá nhân (email, MSSV) chỉ xuất hiện ở R5, C1, C2, B2 và chỉ cho Organizer/Admin trong phạm vi.

### 6.2 Đăng ký (R1)

```sql
BEGIN;
-- 1. Khóa sự kiện: mọi thao tác đổi chỗ của cùng sự kiện xếp hàng tuần tự
SELECT status, capacity, registered_count, waitlist_limit, waitlist_count,
       registration_opens_at, registration_closes_at
FROM events WHERE id = :event_id FOR UPDATE;
-- 2. App kiểm tra: status = 'PUBLISHED' và now() trong cửa sổ đăng ký, nếu không → 422

-- 3a. Còn chỗ (registered_count < capacity)
INSERT INTO registrations (event_id, student_id, status, checkin_code)
VALUES (:event_id, :student_id, 'REGISTERED', :code);
UPDATE events SET registered_count = registered_count + 1 WHERE id = :event_id;

-- 3b. Hết chỗ, waitlist còn chỗ → INSERT status 'WAITLISTED'
--     UPDATE events SET waitlist_count = waitlist_count + 1 ...
-- 3c. Waitlist đầy → ROLLBACK, 409 EVENT_FULL

INSERT INTO notifications (user_id, event_id, type, title, body) VALUES (...);
COMMIT;
-- unique_violation trên uq_active_registration → 409 ALREADY_REGISTERED
```

`checkin_code` sinh sẵn cho cả dòng WAITLISTED nhưng API chỉ hiển thị khi đã REGISTERED; nếu trùng `uq_checkin_code` thì sinh lại và thử lại (xác suất rất thấp với 32^8 tổ hợp).

### 6.3 Hủy + tự promote (R2, E5 tăng capacity)

```sql
BEGIN;
SELECT start_at, capacity, registered_count FROM events WHERE id = :event_id FOR UPDATE;
SELECT status AS old_status FROM registrations WHERE id = :reg_id FOR UPDATE;

UPDATE registrations
SET status = 'CANCELLED', cancelled_at = now(), cancelled_by = :actor,
    cancelled_by_user = :actor_id, cancel_reason = :reason, updated_at = now()
WHERE id = :reg_id AND status IN ('REGISTERED','WAITLISTED')
RETURNING id;
-- old_status = REGISTERED → registered_count - 1; WAITLISTED → waitlist_count - 1

-- Promote người đầu waitlist (chỉ khi now() < start_at và còn chỗ)
WITH head AS (
  SELECT id FROM registrations
  WHERE event_id = :event_id AND status = 'WAITLISTED'
  ORDER BY queue_seq
  LIMIT 1
)
UPDATE registrations r
SET status = 'REGISTERED', promoted_at = now(), updated_at = now()
FROM head WHERE r.id = head.id
RETURNING r.id, r.student_id;
-- Có dòng → registered_count + 1, waitlist_count - 1,
--           INSERT notification 'PROMOTED' (INSERT lỗi thì rollback; gửi email bất đồng bộ sau commit lỗi không rollback — Q14)
COMMIT;
```

Với E5 tăng `capacity` thêm k chỗ, lặp bước promote tối đa k lần trong cùng transaction. `chk_capacity` là chốt chặn cuối: dù code có bug, DB vẫn từ chối mọi trạng thái vượt chỗ.

**Trade-off đã cân nhắc:** khóa dòng `events` đơn giản, dễ chứng minh đúng và đủ nhanh ở quy mô giả định (1.000 chỗ/sự kiện). Phương án thay thế là isolation `SERIALIZABLE` + retry khi xung đột, tránh khóa nhưng phức tạp hơn khi viết và kiểm thử. Bộ đếm `registered_count` thay cho `COUNT(*)` để CHECK constraint chống overbooking được và đọc danh mục không phải đếm lại.

### 6.4 Check-in (C1)

```sql
UPDATE registrations
SET checked_in_at = now(), checkin_method = :method,
    checked_in_by = :staff_id, checkin_note = :note, updated_at = now()
WHERE id = :reg_id AND event_id = :event_id
  AND status = 'REGISTERED'
  AND checked_in_at IS NULL          -- chặn check-in trùng
RETURNING id, checked_in_at;
-- Không có dòng trả về → đọc lại để phân biệt ALREADY_CHECKED_IN và NOT_ELIGIBLE_FOR_CHECKIN
-- method = MANUAL → INSERT activity_logs (action 'MANUAL_CHECKIN') trong cùng transaction
```

Hai máy quét cùng quét một QR thì PostgreSQL khóa dòng registration đó: câu UPDATE thứ hai chờ, đọc lại điều kiện `checked_in_at IS NULL`, thấy đã có giá trị nên không cập nhật — không cần UNIQUE riêng.

Check-in không đụng bộ đếm nên không khóa dòng `events`: nhiều máy quét cùng lúc không chặn nhau. Mỗi lượt là một lần verify HMAC + một câu UPDATE theo khóa chính, phục vụ mục tiêu check-in trung vị ≤ 5 giây (Q11).

### 6.5 Job nền

| Job | Lịch chạy | Việc làm |
| --- | --- | --- |
| Email outbox | Mỗi 30 giây | Gửi notification `email_status = PENDING`; lỗi thì tăng `email_attempts`, tối đa 5 lần rồi đánh dấu FAILED |
| Đóng waitlist | Mỗi 5 phút | Khóa events; sự kiện đã tới `start_at`: WAITLISTED → EXPIRED, waitlist_count = 0, cùng transaction. Đây là đồng bộ dữ liệu lưu, không quyết định thời điểm hết hiệu lực nghiệp vụ |
| Hoàn tất sự kiện | Mỗi 15 phút | PUBLISHED và quá `end_at` → COMPLETED |
| Nhắc lịch | Mỗi giờ | Tạo notification REMINDER cho sự kiện bắt đầu trong 24h tới (bổ trợ, Q1) |

## 7. Truy vết yêu cầu & tiêu chí nghiệm thu

Mỗi mục tiêu thành công thầy đã đồng ý ở Q11 có một chốt chặn trong DB và một endpoint để kiểm thử, nên nhóm có thể viết test nghiệm thu trực tiếp từ bảng dưới.

| Mục tiêu (Q11) | Chốt chặn trong DB | Endpoint | Test nghiệm thu đề xuất |
| --- | --- | --- | --- |
| Không đăng ký trùng | `uq_active_registration` | R1 | Gửi R1 hai lần song song cùng sinh viên → 1 thành công, 1 `409 ALREADY_REGISTERED` |
| Không vượt số chỗ | `chk_capacity` + khóa dòng `events` | R1, E5 | 200 request R1 đồng thời vào sự kiện 50 chỗ → đúng 50 REGISTERED, 150 WAITLISTED |
| Waitlist FIFO | `queue_seq` + `idx_waitlist_head` | R2, R5 | Hủy 3 đăng ký → 3 người đầu waitlist được promote đúng thứ tự `queue_seq` |
| Check-in ≤ 5 giây (trung vị), chặn trùng | Điều kiện `checked_in_at IS NULL` trong câu UPDATE check-in | C1 | Đo 100 lượt quét; quét lại cùng QR → `409 ALREADY_CHECKED_IN` |
| Có phương án thủ công khi QR lỗi | `checkin_method` CODE / MANUAL | C1 | Check-in bằng mã 8 ký tự và bằng xác nhận tay có `note` |
| 500 lượt đăng ký: tổng hợp + xuất ≤ 30 giây, khớp nguồn | `idx_reg_event`, `idx_reg_checked_in` | B1, B2 | Seed 500 đăng ký; đo thời gian B2; so số dòng file với B1 |
| Sự kiện chưa duyệt không công bố được | `event_status` + `chk_no_self_approve` | E1, P3 | Sự kiện PENDING không xuất hiện ở E1; Organizer tự duyệt → `403` |
| Đạt mọi kiểm thử phân quyền | `unit_memberships` | Tất cả | Organizer đơn vị A gọi R5 của đơn vị B → `404`; sinh viên gọi R4 của người khác → `404` |

| Quy tắc khác (nguồn) | Nơi hiện thực |
| --- | --- |
| Hạn hủy 24h (Q13) | R2, `cancelDeadline` trong Event object |
| Gửi email lỗi vẫn giữ đăng ký (Q14) | Outbox `notifications.email_status`, mục 6.3; INSERT outbox lỗi vẫn rollback transaction |
| Đăng ký lại sau hủy, vào cuối waitlist (Q15) | Partial index + `queue_seq` mới cho dòng mới |
| Sửa/hủy sau công bố + notify (Q19) | E5, E7, `activity_logs` (`UPDATE_EVENT`, `CANCEL_EVENT`) |
| Phân quyền xem dữ liệu (Q20) | Mục 6.1 |
| Không check-out, không SSO ngoài Google, không app native | Không có bảng/endpoint tương ứng; Google OAuth 2.0 / OIDC thay login mô phỏng |

**Đã chốt thêm (22/10):** phạm vi Organizer tính theo đơn vị; promote muộn và thay đổi quan trọng trong 24h cuối đều giữ quy tắc 24h, hủy qua Organizer. Chi tiết ở phần giả định, mục 1.
