-- A25 initial database schema: PostgreSQL 16+
-- Apply once to a NEW database. For existing deployed databases use ALTER migrations.
-- Baseline 24/10/2026; see docs/db-schema-and-api-contract.md. Keep DDL in sync.
BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;  -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS citext;

-- ========== ENUMS ==========
-- Vai trò người dùng KHÔNG lưu bằng enum: USER chuyên biệt hóa chồng lấn (EERD, phương án ánh xạ 8D)
--   STUDENT   ⇔ users.student_code IS NOT NULL
--   ORGANIZER ⇔ có dòng trong unit_memberships
--   ADMIN     ⇔ users.is_admin = true
CREATE TYPE unit_type           AS ENUM ('FACULTY','CLUB','DEPARTMENT');
CREATE TYPE membership_role     AS ENUM ('ORGANIZER');
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
  student_code  varchar(10) UNIQUE,              -- MSSV; NOT NULL ⇔ là STUDENT; NULL với cán bộ
  is_admin      boolean NOT NULL DEFAULT false,  -- chỉ đội dev cấp qua quy trình nội bộ
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
  member_role membership_role NOT NULL,          -- chỉ ORGANIZER; Admin cấp trường không cần membership
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
  cancellation_exception_until timestamptz,             -- ngoại lệ Q13–Q19, chỉ cấp cho đăng ký có trước thay đổi
  cancelled_by      cancel_actor,
  cancelled_by_user uuid REFERENCES users(id),
  cancel_reason     varchar(255),
  checkin_code      char(8) NOT NULL,                    -- mã chuẩn hóa 8 ký tự; API format XXXX-XXXX
  -- check-in (gộp từ bảng check_ins cũ)
  checked_in_at     timestamptz,
  checkin_method    checkin_method,
  checked_in_by     uuid REFERENCES users(id),
  checkin_note      varchar(255),                        -- bắt buộc khi MANUAL
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT uq_checkin_code UNIQUE (event_id, checkin_code),
  CONSTRAINT chk_cancel_exception CHECK (cancellation_exception_until IS NULL OR status = 'REGISTERED'),
  CONSTRAINT chk_cancel_fields CHECK (
    (status IN ('CANCELLED','CANCELLED_BY_EVENT')) = (cancelled_at IS NOT NULL)),
  CONSTRAINT chk_checkin_fields CHECK (
    (checked_in_at IS NULL) = (checkin_method IS NULL) AND
    (checked_in_at IS NULL) = (checked_in_by  IS NULL)),
  CONSTRAINT chk_manual_note CHECK (
    checkin_method IS DISTINCT FROM 'MANUAL' OR checkin_note IS NOT NULL),
  CONSTRAINT chk_no_self_checkin CHECK (checked_in_by IS NULL OR checked_in_by <> student_id)  -- Organizer kiêm sinh viên
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
-- Ngoại lệ hủy Q13–Q19
CREATE INDEX idx_reg_cancel_exception ON registrations (event_id, cancellation_exception_until)
  WHERE cancellation_exception_until IS NOT NULL;

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

COMMIT;
