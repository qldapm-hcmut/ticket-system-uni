-- Local-only demo identities for DEV_LOGIN_ENABLED=true.
-- Run after migrations with: psql "$DATABASE_URL" -f db/seed/demo.sql
BEGIN;

INSERT INTO units (code, name, type)
VALUES ('DEMO', 'Demo Event Unit', 'FACULTY')
ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, is_active = true;

INSERT INTO users (email, full_name, student_code, is_admin)
VALUES
  ('admin.demo@hcmut.edu.vn', 'Demo Admin', NULL, true),
  ('organizer.demo@hcmut.edu.vn', 'Demo Organizer', NULL, false),
  ('student.demo@hcmut.edu.vn', 'Demo Student', '2210001', false)
ON CONFLICT (email) DO UPDATE SET
  full_name = EXCLUDED.full_name,
  student_code = EXCLUDED.student_code,
  is_admin = EXCLUDED.is_admin,
  is_active = true;

INSERT INTO unit_memberships (user_id, unit_id, member_role)
SELECT u.id, un.id, 'ORGANIZER'
FROM users u CROSS JOIN units un
WHERE u.email = 'organizer.demo@hcmut.edu.vn' AND un.code = 'DEMO'
ON CONFLICT (user_id, unit_id, member_role) DO NOTHING;

COMMIT;
