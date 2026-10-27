-- psql script; caller must supply -v admin_email='verified-person@hcmut.edu.vn'.
-- Does not grant Admin to an existing user: elevation requires a separate audited process.
\set ON_ERROR_STOP on
BEGIN;
INSERT INTO units (code, name, type)
VALUES ('CSE', 'Khoa Khoa học và Kỹ thuật Máy tính', 'FACULTY')
ON CONFLICT (code) DO NOTHING;

INSERT INTO users (email, full_name, is_admin)
VALUES (:'admin_email', 'Bootstrap Admin', true)
ON CONFLICT (email) DO NOTHING;

-- Only audit a newly bootstrapped identity once. No Google identity is pre-linked.
INSERT INTO activity_logs (entity_type, entity_id, action, metadata)
SELECT 'USER', id, 'GRANT_ADMIN', '{"ticket":"initial-bootstrap"}'::jsonb
FROM users u WHERE email = :'admin_email' AND is_admin = true
  AND NOT EXISTS (SELECT 1 FROM activity_logs a WHERE a.entity_id = u.id AND a.action = 'GRANT_ADMIN');
COMMIT;
