BEGIN;

CREATE TABLE version (
	singleton boolean PRIMARY KEY DEFAULT TRUE CHECK (singleton),
	version integer NOT NULL
);

INSERT INTO version (singleton, version)
VALUES (TRUE, 1)
ON CONFLICT (singleton) DO UPDATE SET version = EXCLUDED.version;

CREATE TABLE users (
	id bigserial PRIMARY KEY,
	username text UNIQUE NOT NULL,
	password_hash text NOT NULL,
	is_admin boolean NOT NULL DEFAULT FALSE,
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
	token text PRIMARY KEY,
	user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at timestamptz NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TYPE ticket_status AS ENUM ('waiting_on_admin', 'waiting_on_user', 'closed');

CREATE TABLE tickets (
	id bigserial PRIMARY KEY,
	owner_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	title text NOT NULL,
	status ticket_status NOT NULL DEFAULT 'waiting_on_admin',
	last_activity_at timestamptz NOT NULL DEFAULT now(),
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ticket_messages (
	id bigserial PRIMARY KEY,
	ticket_id bigint NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
	author_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	body text NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ticket_attachments (
	id bigserial PRIMARY KEY,
	message_id bigint NOT NULL REFERENCES ticket_messages(id) ON DELETE CASCADE,
	original_name text NOT NULL,
	stored_name text NOT NULL,
	content_type text,
	created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_tickets_owner ON tickets(owner_id);
CREATE INDEX idx_tickets_status ON tickets(status);
CREATE INDEX idx_tickets_last_activity ON tickets(last_activity_at DESC);
CREATE INDEX idx_ticket_messages_ticket ON ticket_messages(ticket_id);
CREATE INDEX idx_ticket_attachments_message ON ticket_attachments(message_id);

COMMIT;
