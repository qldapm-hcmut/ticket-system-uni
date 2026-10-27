-- DEV/TEST ONLY: destroys all A25 data.
BEGIN;
DROP TABLE IF EXISTS activity_logs CASCADE;
DROP TABLE IF EXISTS auth_sessions CASCADE;
DROP TABLE IF EXISTS notifications CASCADE;
DROP TABLE IF EXISTS registrations CASCADE;
DROP TABLE IF EXISTS events CASCADE;
DROP TABLE IF EXISTS unit_memberships CASCADE;
DROP TABLE IF EXISTS units CASCADE;
DROP TABLE IF EXISTS users CASCADE;
DROP TYPE IF EXISTS delivery_status;
DROP TYPE IF EXISTS notification_type;
DROP TYPE IF EXISTS checkin_method;
DROP TYPE IF EXISTS cancel_actor;
DROP TYPE IF EXISTS registration_status;
DROP TYPE IF EXISTS event_status;
DROP TYPE IF EXISTS membership_role;
DROP TYPE IF EXISTS unit_type;
DROP TYPE IF EXISTS user_role;  -- chỉ còn trong DB tạo từ bản 0001 cũ (trước 24/10); giữ để dọn sạch
COMMIT;
