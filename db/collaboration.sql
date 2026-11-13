-- Executed with schema.sql in the same implicit PostgreSQL transaction.
CREATE TABLE IF NOT EXISTS local_users (
 id BIGSERIAL PRIMARY KEY, username TEXT NOT NULL UNIQUE CHECK(length(username) BETWEEN 1 AND 80),
 display_name TEXT NOT NULL, password_hash TEXT NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT true, admin BOOLEAN NOT NULL DEFAULT false,
 preferences JSONB NOT NULL DEFAULT '{}', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS user_sessions (
 id UUID PRIMARY KEY, user_id BIGINT NOT NULL REFERENCES local_users(id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), expires_at TIMESTAMPTZ NOT NULL,
 revoked_at TIMESTAMPTZ, user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user ON user_sessions(user_id, expires_at);
CREATE TABLE IF NOT EXISTS engagements (
 id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
 company_id BIGINT REFERENCES companies(id) ON DELETE SET NULL,
 starts_on DATE, ends_on DATE, status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('planned','active','completed','archived')),
 settings JSONB NOT NULL DEFAULT '{}', legacy BOOLEAN NOT NULL DEFAULT false,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), CHECK(ends_on IS NULL OR starts_on IS NULL OR ends_on>=starts_on)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_engagement_legacy ON engagements(legacy) WHERE legacy;
INSERT INTO engagements(name,description,legacy) SELECT 'General','Tasks and chats not assigned to an engagement',true
 WHERE NOT EXISTS(SELECT 1 FROM engagements WHERE legacy);
-- Rename the original seed only while it still carries the old default text.
UPDATE engagements SET name='General',description='Tasks and chats not assigned to an engagement'
 WHERE legacy AND name='Legacy private workspace' AND description='Preserved pre-collaboration records';
CREATE TABLE IF NOT EXISTS engagement_members (
 engagement_id BIGINT NOT NULL REFERENCES engagements(id), user_id BIGINT NOT NULL REFERENCES local_users(id),
 role TEXT NOT NULL CHECK(role IN ('manager','operator','viewer')), PRIMARY KEY(engagement_id,user_id)
);
CREATE INDEX IF NOT EXISTS idx_members_user ON engagement_members(user_id,engagement_id);
CREATE OR REPLACE FUNCTION legacy_engagement_id() RETURNS BIGINT LANGUAGE sql STABLE AS
 $$ SELECT id FROM engagements WHERE legacy $$;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS engagement_id BIGINT REFERENCES engagements(id);
UPDATE tasks SET engagement_id=legacy_engagement_id() WHERE engagement_id IS NULL;
ALTER TABLE tasks ALTER COLUMN engagement_id SET DEFAULT legacy_engagement_id();
ALTER TABLE tasks ALTER COLUMN engagement_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_tasks_engagement ON tasks(engagement_id,created_at DESC,id DESC);
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS engagement_id BIGINT REFERENCES engagements(id);
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS owner_id BIGINT REFERENCES local_users(id);
UPDATE conversations SET engagement_id=legacy_engagement_id() WHERE engagement_id IS NULL;

-- Copy the existing hash verbatim once. Never overwrite a named account on startup.
INSERT INTO local_users(username,display_name,password_hash,admin)
 SELECT 'ARTEX','ARTEX',value,true FROM settings WHERE key='auth.password_hash' AND value<>''
 ON CONFLICT(username) DO NOTHING;
INSERT INTO engagement_members(engagement_id,user_id,role)
 SELECT legacy_engagement_id(),id,'manager' FROM local_users WHERE username='ARTEX' AND admin
 ON CONFLICT DO NOTHING;
UPDATE conversations SET owner_id=(SELECT id FROM local_users WHERE username='ARTEX' AND admin) WHERE owner_id IS NULL;

CREATE TABLE IF NOT EXISTS collaboration_audit (
 id BIGSERIAL PRIMARY KEY, actor_id BIGINT REFERENCES local_users(id), engagement_id BIGINT REFERENCES engagements(id),
 action TEXT NOT NULL, subject TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_collaboration_audit ON collaboration_audit(engagement_id,id DESC);
CREATE TABLE IF NOT EXISTS saved_reports (
 id BIGSERIAL PRIMARY KEY, engagement_id BIGINT NOT NULL REFERENCES engagements(id),
 family_id UUID NOT NULL, version INTEGER NOT NULL, author_id BIGINT NOT NULL REFERENCES local_users(id),
 title TEXT NOT NULL, task_ids JSONB NOT NULL, snapshot JSONB NOT NULL, source_hash TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','running','failed','completed','canceled')),
 profile_id BIGINT NOT NULL DEFAULT 0, model TEXT NOT NULL, prompt_version TEXT NOT NULL, usage JSONB NOT NULL DEFAULT '{}',
 error TEXT NOT NULL DEFAULT '', output TEXT NOT NULL DEFAULT '',
 reviewer_id BIGINT REFERENCES local_users(id), approved_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), finished_at TIMESTAMPTZ,
 UNIQUE(family_id,version)
);
CREATE INDEX IF NOT EXISTS idx_saved_reports_list ON saved_reports(engagement_id,id DESC);
CREATE TABLE IF NOT EXISTS discussion_threads (
 id BIGSERIAL PRIMARY KEY, engagement_id BIGINT NOT NULL REFERENCES engagements(id),
 owner_id BIGINT NOT NULL REFERENCES local_users(id), title TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('discussion','shared_ai','private_ai')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_threads_engagement ON discussion_threads(engagement_id,id DESC);
CREATE TABLE IF NOT EXISTS discussion_messages (
 id BIGSERIAL PRIMARY KEY, thread_id BIGINT NOT NULL REFERENCES discussion_threads(id),
 author_id BIGINT NOT NULL REFERENCES local_users(id), client_id UUID NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('human','ai')), body TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'completed' CHECK(state IN ('queued','running','completed','failed','canceled')),
 error TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(thread_id,client_id)
);
CREATE INDEX IF NOT EXISTS idx_messages_thread ON discussion_messages(thread_id,id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_thread_ai_active ON discussion_messages(thread_id) WHERE kind='ai' AND state IN ('queued','running');
CREATE TABLE IF NOT EXISTS discussion_attachments (
 id UUID PRIMARY KEY, thread_id BIGINT NOT NULL REFERENCES discussion_threads(id),
 author_id BIGINT NOT NULL REFERENCES local_users(id), name TEXT NOT NULL, body BYTEA NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION protect_report_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.engagement_id,NEW.family_id,NEW.version,NEW.author_id,NEW.title,NEW.task_ids,NEW.snapshot,NEW.source_hash,NEW.model,NEW.profile_id,NEW.prompt_version)
 IS DISTINCT FROM ROW(OLD.engagement_id,OLD.family_id,OLD.version,OLD.author_id,OLD.title,OLD.task_ids,OLD.snapshot,OLD.source_hash,OLD.model,OLD.profile_id,OLD.prompt_version)
 THEN RAISE EXCEPTION 'Report sources are immutable'; END IF;
 IF OLD.status IN ('completed','failed','canceled') AND ROW(NEW.status,NEW.output,NEW.error,NEW.usage,NEW.finished_at) IS DISTINCT FROM ROW(OLD.status,OLD.output,OLD.error,OLD.usage,OLD.finished_at)
 THEN RAISE EXCEPTION 'Report output is immutable'; END IF;
 IF OLD.approved_at IS NOT NULL AND ROW(NEW.reviewer_id,NEW.approved_at) IS DISTINCT FROM ROW(OLD.reviewer_id,OLD.approved_at)
 THEN RAISE EXCEPTION 'Report approval is immutable'; END IF;
 IF NEW.approved_at IS NOT NULL AND (NEW.status<>'completed' OR NEW.reviewer_id IS NULL) THEN RAISE EXCEPTION 'Only completed reports may be approved'; END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS protect_saved_report ON saved_reports;
CREATE TRIGGER protect_saved_report BEFORE UPDATE ON saved_reports FOR EACH ROW EXECUTE FUNCTION protect_report_version();

ALTER TABLE discussion_messages ADD COLUMN IF NOT EXISTS attachment_id UUID REFERENCES discussion_attachments(id);
ALTER TABLE discussion_messages ADD COLUMN IF NOT EXISTS context_json JSONB NOT NULL DEFAULT '{}';
ALTER TABLE discussion_messages ADD COLUMN IF NOT EXISTS usage JSONB NOT NULL DEFAULT '{}';
ALTER TABLE discussion_messages ADD COLUMN IF NOT EXISTS model TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS discussion_events (
 id BIGSERIAL PRIMARY KEY, thread_id BIGINT NOT NULL REFERENCES discussion_threads(id),
 message_id BIGINT NOT NULL REFERENCES discussion_messages(id), created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_discussion_events ON discussion_events(thread_id,id);
CREATE OR REPLACE FUNCTION append_discussion_event() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN INSERT INTO discussion_events(thread_id,message_id) VALUES(NEW.thread_id,NEW.id); RETURN NEW; END $$;
DROP TRIGGER IF EXISTS discussion_event ON discussion_messages;
CREATE TRIGGER discussion_event AFTER INSERT OR UPDATE ON discussion_messages FOR EACH ROW EXECUTE FUNCTION append_discussion_event();

CREATE OR REPLACE FUNCTION enforce_engagement_relation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (SELECT engagement_id FROM tasks WHERE id=NEW.task_id) IS DISTINCT FROM (SELECT engagement_id FROM tasks WHERE id=NEW.source_task_id)
 THEN RAISE EXCEPTION 'Source tasks must belong to the same engagement'; END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS engagement_relation ON task_relations;
CREATE TRIGGER engagement_relation BEFORE INSERT OR UPDATE ON task_relations FOR EACH ROW EXECUTE FUNCTION enforce_engagement_relation();
