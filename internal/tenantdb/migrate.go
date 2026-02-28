package tenantdb

import (
	"database/sql"
	"fmt"
	"strings"
)

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS workspaces (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  email TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  last_login_at TEXT
);

CREATE TABLE IF NOT EXISTS memberships (
  workspace_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  is_owner INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  PRIMARY KEY(workspace_id, user_id),
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id),
  FOREIGN KEY(user_id) REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS invites (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id INTEGER NOT NULL,
  token_hash BLOB NOT NULL UNIQUE,
  email TEXT NOT NULL,
  created_by_user_id INTEGER NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  expires_at TEXT NOT NULL,
  redeemed_at TEXT,
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id),
  FOREIGN KEY(created_by_user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_invites_workspace
  ON invites(workspace_id, created_at DESC);

CREATE TABLE IF NOT EXISTS contacts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  email TEXT NOT NULL DEFAULT '',
  phone TEXT NOT NULL DEFAULT '',
  company TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id)
);

CREATE INDEX IF NOT EXISTS idx_contacts_workspace_name
  ON contacts(workspace_id, name);

CREATE VIRTUAL TABLE IF NOT EXISTS contacts_fts USING fts5(
  name,
  email,
  phone,
  company,
  notes,
  content='contacts',
  content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS contacts_ai AFTER INSERT ON contacts BEGIN
  INSERT INTO contacts_fts(rowid, name, email, phone, company, notes)
  VALUES (new.id, new.name, new.email, new.phone, new.company, new.notes);
END;

CREATE TRIGGER IF NOT EXISTS contacts_ad AFTER DELETE ON contacts BEGIN
  INSERT INTO contacts_fts(contacts_fts, rowid, name, email, phone, company, notes)
  VALUES ('delete', old.id, old.name, old.email, old.phone, old.company, old.notes);
END;

CREATE TRIGGER IF NOT EXISTS contacts_au AFTER UPDATE ON contacts BEGIN
  INSERT INTO contacts_fts(contacts_fts, rowid, name, email, phone, company, notes)
  VALUES ('delete', old.id, old.name, old.email, old.phone, old.company, old.notes);
  INSERT INTO contacts_fts(rowid, name, email, phone, company, notes)
  VALUES (new.id, new.name, new.email, new.phone, new.company, new.notes);
END;

CREATE TABLE IF NOT EXISTS interactions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id INTEGER NOT NULL,
  contact_id INTEGER NOT NULL,
  type TEXT NOT NULL CHECK(type IN ('note','call','email','meeting')),
  content TEXT NOT NULL,
  due_at TEXT,
  completed_at TEXT,
  created_by_user_id INTEGER,
  updated_by_user_id INTEGER,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id),
  FOREIGN KEY(contact_id) REFERENCES contacts(id),
  FOREIGN KEY(created_by_user_id) REFERENCES users(id),
  FOREIGN KEY(updated_by_user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_interactions_workspace_created
  ON interactions(workspace_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_interactions_workspace_due
  ON interactions(workspace_id, due_at, completed_at);

CREATE TABLE IF NOT EXISTS deals (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id INTEGER NOT NULL,
  title TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'open' CHECK(state IN ('open','won','lost')),
  value_cents INTEGER,
  stage_label TEXT NOT NULL DEFAULT '',
  next_step TEXT NOT NULL DEFAULT '',
  next_step_due_at TEXT,
  next_step_completed_at TEXT,
  close_window_start TEXT,
  close_window_end TEXT,
  closed_at TEXT,
  closed_outcome TEXT NOT NULL DEFAULT '',
  last_activity_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id)
);

CREATE INDEX IF NOT EXISTS idx_deals_workspace_state
  ON deals(workspace_id, state, last_activity_at DESC);

CREATE INDEX IF NOT EXISTS idx_deals_workspace_next_step_due
  ON deals(workspace_id, state, next_step_due_at, next_step_completed_at);

CREATE TABLE IF NOT EXISTS ledger_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id INTEGER NOT NULL,
  event_version INTEGER NOT NULL DEFAULT 1,

  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),

  actor_kind TEXT NOT NULL CHECK(actor_kind IN ('human','agent','system')),
  actor_user_id INTEGER,

  op TEXT NOT NULL,
  entity_type TEXT NOT NULL,
  entity_id INTEGER,

  payload_json TEXT NOT NULL DEFAULT '',
  reason TEXT NOT NULL DEFAULT '',
  evidence_json TEXT NOT NULL DEFAULT '',

  caused_by_event_id INTEGER,
  replaces_event_id INTEGER,
  inverse_of_event_id INTEGER,

  idempotency_key TEXT,

  FOREIGN KEY(workspace_id) REFERENCES workspaces(id),
  FOREIGN KEY(actor_user_id) REFERENCES users(id),
  FOREIGN KEY(caused_by_event_id) REFERENCES ledger_events(id),
  FOREIGN KEY(replaces_event_id) REFERENCES ledger_events(id),
  FOREIGN KEY(inverse_of_event_id) REFERENCES ledger_events(id)
);

CREATE INDEX IF NOT EXISTS idx_ledger_events_workspace_created
  ON ledger_events(workspace_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_ledger_events_workspace_actor_created
  ON ledger_events(workspace_id, actor_kind, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_ledger_events_entity
  ON ledger_events(workspace_id, entity_type, entity_id, created_at DESC, id DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ledger_events_idempotency
  ON ledger_events(workspace_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL AND idempotency_key != '';

CREATE TABLE IF NOT EXISTS projection_cursors (
  workspace_id INTEGER NOT NULL,
  projection_name TEXT NOT NULL,
  last_event_id INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  PRIMARY KEY(workspace_id, projection_name),
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id)
);

CREATE TABLE IF NOT EXISTS entity_id_counters (
  workspace_id INTEGER NOT NULL,
  entity_type TEXT NOT NULL,
  next_id INTEGER NOT NULL DEFAULT 1,
  PRIMARY KEY(workspace_id, entity_type),
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id)
);

CREATE TABLE IF NOT EXISTS activity_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id INTEGER NOT NULL,
  actor_kind TEXT NOT NULL CHECK(actor_kind IN ('human','agent','system')),
  actor_user_id INTEGER,
  verb TEXT NOT NULL DEFAULT '',
  object_type TEXT NOT NULL DEFAULT '',
  object_id INTEGER,
  status TEXT NOT NULL DEFAULT 'done' CHECK(status IN ('done','current','error','canceled','paused','staged','proposed')),
  title TEXT NOT NULL,
  summary TEXT NOT NULL DEFAULT '',
  detail_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id),
  FOREIGN KEY(actor_user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_activity_events_workspace_created
  ON activity_events(workspace_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_activity_events_workspace_actor_status
  ON activity_events(workspace_id, actor_kind, status, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS deal_contacts (
  deal_id INTEGER NOT NULL,
  contact_id INTEGER NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  PRIMARY KEY(deal_id, contact_id),
  FOREIGN KEY(deal_id) REFERENCES deals(id) ON DELETE CASCADE,
  FOREIGN KEY(contact_id) REFERENCES contacts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_deal_contacts_contact
  ON deal_contacts(contact_id, deal_id);

CREATE TABLE IF NOT EXISTS deal_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id INTEGER NOT NULL,
  deal_id INTEGER NOT NULL,
  type TEXT NOT NULL CHECK(type IN ('note','call','email','meeting','system')),
  content TEXT NOT NULL,
  created_by_user_id INTEGER,
  updated_by_user_id INTEGER,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  FOREIGN KEY(workspace_id) REFERENCES workspaces(id),
  FOREIGN KEY(deal_id) REFERENCES deals(id) ON DELETE CASCADE,
  FOREIGN KEY(created_by_user_id) REFERENCES users(id),
  FOREIGN KEY(updated_by_user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_deal_events_deal_created
  ON deal_events(workspace_id, deal_id, created_at DESC);

CREATE VIRTUAL TABLE IF NOT EXISTS interactions_fts USING fts5(
  content,
  content='interactions',
  content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS interactions_ai AFTER INSERT ON interactions BEGIN
  INSERT INTO interactions_fts(rowid, content) VALUES (new.id, new.content);
END;

CREATE TRIGGER IF NOT EXISTS interactions_ad AFTER DELETE ON interactions BEGIN
  INSERT INTO interactions_fts(interactions_fts, rowid, content) VALUES ('delete', old.id, old.content);
END;

CREATE TRIGGER IF NOT EXISTS interactions_au AFTER UPDATE ON interactions BEGIN
  INSERT INTO interactions_fts(interactions_fts, rowid, content) VALUES ('delete', old.id, old.content);
  INSERT INTO interactions_fts(rowid, content) VALUES (new.id, new.content);
END;

CREATE TABLE IF NOT EXISTS passkey_credentials (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  credential_id BLOB NOT NULL UNIQUE,
  credential_json TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  FOREIGN KEY(user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_passkey_credentials_user
  ON passkey_credentials(user_id);
`)
	if err != nil {
		return fmt.Errorf("migrate tenant: %w", err)
	}
	// Add newer invite fields for safe multi-step redemption.
	_ = execAddColumn(db, "invites", "redeem_started_at TEXT")
	_ = execAddColumn(db, "invites", "redeem_user_id INTEGER")
	_ = execAddColumn(db, "invites", "revoked_at TEXT")
	_ = execAddColumn(db, "interactions", "created_by_user_id INTEGER")
	_ = execAddColumn(db, "interactions", "updated_by_user_id INTEGER")
	_ = execAddColumn(db, "deal_events", "created_by_user_id INTEGER")
	_ = execAddColumn(db, "deal_events", "updated_by_user_id INTEGER")
	return nil
}

func execAddColumn(db *sql.DB, table, columnDef string) error {
	_, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + columnDef)
	if err != nil && strings.Contains(err.Error(), "duplicate column name") {
		return nil
	}
	return err
}

func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return fmt.Errorf("apply pragma %q: %w", p, err)
		}
	}
	return nil
}
