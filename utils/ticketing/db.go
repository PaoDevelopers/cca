package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TicketStatus string

const (
	StatusWaitingOnAdmin TicketStatus = "waiting_on_admin"
	StatusWaitingOnUser  TicketStatus = "waiting_on_user"
	StatusClosed         TicketStatus = "closed"
)

var statusSet = map[TicketStatus]struct{}{
	StatusWaitingOnAdmin: {},
	StatusWaitingOnUser:  {},
	StatusClosed:         {},
}

func ValidateStatus(status TicketStatus) error {
	if _, ok := statusSet[status]; !ok {
		return fmt.Errorf("invalid status %q", status)
	}
	return nil
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Close() {
	s.pool.Close()
}

const schemaVersion = 1

func (s *Store) CheckVersion(ctx context.Context) error {
	var version int
	err := s.pool.QueryRow(ctx, `SELECT version FROM version WHERE singleton`).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("schema version row missing")
	}
	if err != nil {
		return fmt.Errorf("check schema version: %w", err)
	}
	if version != schemaVersion {
		return fmt.Errorf("schema version mismatch: expected %d got %d", schemaVersion, version)
	}
	return nil
}

type User struct {
	ID        int64
	Username  string
	Password  string
	IsAdmin   bool
	CreatedAt time.Time
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash string, isAdmin bool) (*User, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, is_admin)
		VALUES ($1, $2, $3)
		RETURNING id, username, password_hash, is_admin, created_at
	`, username, passwordHash, isAdmin)
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Password, &u.IsAdmin, &u.CreatedAt); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &u, nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, username, password_hash, is_admin, created_at
		FROM users
		WHERE username = $1
	`, username)
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.Password, &u.IsAdmin, &u.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &u, nil
}

type Session struct {
	Token     string
	UserID    int64
	ExpiresAt time.Time
	CreatedAt time.Time
	User      User
}

func (s *Store) CreateSession(ctx context.Context, token string, userID int64, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (token, user_id, expires_at)
		VALUES ($1, $2, $3)
	`, token, userID, expiresAt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (s *Store) GetSessionWithUser(ctx context.Context, token string, now time.Time) (*Session, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT s.token, s.user_id, s.expires_at, s.created_at,
		       u.id, u.username, u.password_hash, u.is_admin, u.created_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token = $1
	`, token)
	var sess Session
	if err := row.Scan(
		&sess.Token,
		&sess.UserID,
		&sess.ExpiresAt,
		&sess.CreatedAt,
		&sess.User.ID,
		&sess.User.Username,
		&sess.User.Password,
		&sess.User.IsAdmin,
		&sess.User.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get session: %w", err)
	}
	if !sess.ExpiresAt.After(now) {
		return nil, nil
	}
	return &sess, nil
}

func (s *Store) RenewSession(ctx context.Context, token string, expires time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE sessions SET expires_at = $2 WHERE token = $1
	`, token, expires)
	if err != nil {
		return fmt.Errorf("renew session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("session not found for renewal")
	}
	return nil
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, token)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

type Ticket struct {
	ID             int64
	OwnerID        int64
	OwnerUsername  string
	Title          string
	Status         TicketStatus
	LastActivityAt time.Time
	CreatedAt      time.Time
}

type Message struct {
	ID        int64
	TicketID  int64
	AuthorID  int64
	Author    string
	Body      string
	CreatedAt time.Time
}

type Attachment struct {
	ID           int64
	MessageID    int64
	StoredName   string
	OriginalName string
	ContentType  *string
	CreatedAt    time.Time
}

type MessageWithAttachments struct {
	Message
	Attachments []Attachment
}

type AttachmentInput struct {
	StoredName   string
	OriginalName string
	ContentType  *string
}

func (s *Store) CreateTicketWithMessage(ctx context.Context, ownerID int64, title string, status TicketStatus, body string, attachments []AttachmentInput) (*Ticket, *Message, error) {
	if err := ValidateStatus(status); err != nil {
		return nil, nil, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var ticket Ticket
	err = tx.QueryRow(ctx, `
		INSERT INTO tickets (owner_id, title, status)
		VALUES ($1, $2, $3)
		RETURNING id, owner_id, title, status, last_activity_at, created_at
	`, ownerID, title, status).Scan(
		&ticket.ID,
		&ticket.OwnerID,
		&ticket.Title,
		&ticket.Status,
		&ticket.LastActivityAt,
		&ticket.CreatedAt,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("insert ticket: %w", err)
	}

	var msg Message
	err = tx.QueryRow(ctx, `
		INSERT INTO ticket_messages (ticket_id, author_id, body)
		VALUES ($1, $2, $3)
		RETURNING id, ticket_id, author_id, body, created_at
	`, ticket.ID, ownerID, body).Scan(
		&msg.ID,
		&msg.TicketID,
		&msg.AuthorID,
		&msg.Body,
		&msg.CreatedAt,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("insert message: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE tickets SET last_activity_at = $2 WHERE id = $1`, ticket.ID, msg.CreatedAt); err != nil {
		return nil, nil, fmt.Errorf("update last activity: %w", err)
	}

	if len(attachments) > 0 {
		if err := insertAttachments(ctx, tx, msg.ID, attachments); err != nil {
			return nil, nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("commit ticket: %w", err)
	}
	return &ticket, &msg, nil
}

func (s *Store) AddMessage(ctx context.Context, ticketID, authorID int64, body string, attachments []AttachmentInput) (*Message, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin message tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	row := tx.QueryRow(ctx, `
		INSERT INTO ticket_messages (ticket_id, author_id, body)
		VALUES ($1, $2, $3)
		RETURNING id, ticket_id, author_id, body, created_at
	`, ticketID, authorID, body)

	var msg Message
	if err := row.Scan(&msg.ID, &msg.TicketID, &msg.AuthorID, &msg.Body, &msg.CreatedAt); err != nil {
		return nil, fmt.Errorf("insert message: %w", err)
	}

	if _, err := tx.Exec(ctx, `UPDATE tickets SET last_activity_at = $2 WHERE id = $1`, ticketID, msg.CreatedAt); err != nil {
		return nil, fmt.Errorf("update last activity: %w", err)
	}

	if len(attachments) > 0 {
		if err := insertAttachments(ctx, tx, msg.ID, attachments); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit message: %w", err)
	}
	return &msg, nil
}

func insertAttachments(ctx context.Context, tx pgx.Tx, messageID int64, attachments []AttachmentInput) error {
	for _, att := range attachments {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ticket_attachments (message_id, stored_name, original_name, content_type)
			VALUES ($1, $2, $3, $4)
		`, messageID, att.StoredName, att.OriginalName, att.ContentType); err != nil {
			return fmt.Errorf("insert attachment: %w", err)
		}
	}
	return nil
}

func (s *Store) UpdateTicketStatus(ctx context.Context, ticketID int64, status TicketStatus) error {
	if err := ValidateStatus(status); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE tickets
		SET status = $2, last_activity_at = now()
		WHERE id = $1
	`, ticketID, status)
	if err != nil {
		return fmt.Errorf("update ticket status: %w", err)
	}
	return nil
}

func (s *Store) UpdateTicketTitle(ctx context.Context, ticketID int64, title string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE tickets
		SET title = $2, last_activity_at = now()
		WHERE id = $1
	`, ticketID, title)
	if err != nil {
		return fmt.Errorf("update ticket title: %w", err)
	}
	return nil
}

func (s *Store) GetTicket(ctx context.Context, ticketID int64) (*Ticket, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT t.id, t.owner_id, u.username, t.title, t.status, t.last_activity_at, t.created_at
		FROM tickets t
		JOIN users u ON u.id = t.owner_id
		WHERE t.id = $1
	`, ticketID)
	var t Ticket
	if err := row.Scan(&t.ID, &t.OwnerID, &t.OwnerUsername, &t.Title, &t.Status, &t.LastActivityAt, &t.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get ticket: %w", err)
	}
	return &t, nil
}

func (s *Store) GetTicketMessages(ctx context.Context, ticketID int64) ([]MessageWithAttachments, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT m.id, m.ticket_id, m.author_id, u.username, m.body, m.created_at
		FROM ticket_messages m
		JOIN users u ON u.id = m.author_id
		WHERE m.ticket_id = $1
		ORDER BY m.created_at ASC
	`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("query ticket messages: %w", err)
	}
	defer rows.Close()

	var messages []MessageWithAttachments
	for rows.Next() {
		var msg MessageWithAttachments
		if err := rows.Scan(&msg.ID, &msg.TicketID, &msg.AuthorID, &msg.Author, &msg.Body, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ticket message: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ticket messages: %w", err)
	}

	for i := range messages {
		attachments, err := s.getAttachmentsForMessage(ctx, messages[i].ID)
		if err != nil {
			return nil, err
		}
		messages[i].Attachments = attachments
	}
	return messages, nil
}

func (s *Store) getAttachmentsForMessage(ctx context.Context, messageID int64) ([]Attachment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, message_id, stored_name, original_name, content_type, created_at
		FROM ticket_attachments
		WHERE message_id = $1
		ORDER BY created_at ASC
	`, messageID)
	if err != nil {
		return nil, fmt.Errorf("query attachments: %w", err)
	}
	defer rows.Close()

	var attachments []Attachment
	for rows.Next() {
		var att Attachment
		var contentType *string
		if err := rows.Scan(&att.ID, &att.MessageID, &att.StoredName, &att.OriginalName, &contentType, &att.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan attachment: %w", err)
		}
		att.ContentType = contentType
		attachments = append(attachments, att)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attachments: %w", err)
	}
	return attachments, nil
}

func (s *Store) ListTicketsByStatus(ctx context.Context, status TicketStatus, userID int64, includeAll bool, search string) ([]Ticket, error) {
	if err := ValidateStatus(status); err != nil {
		return nil, err
	}

	args := []any{status}
	clauses := []string{"t.status = $1"}

	if !includeAll {
		args = append(args, userID)
		clauses = append(clauses, fmt.Sprintf("t.owner_id = $%d", len(args)))
	}

	if search != "" {
		pattern := "%" + search + "%"
		args = append(args, pattern)
		titleIdx := len(args)
		titleClause := fmt.Sprintf("LOWER(t.title) LIKE LOWER($%d)", titleIdx)
		if includeAll {
			args = append(args, pattern)
			userIdx := len(args)
			clauses = append(clauses, fmt.Sprintf("(%s OR LOWER(u.username) LIKE LOWER($%d))", titleClause, userIdx))
		} else {
			clauses = append(clauses, titleClause)
		}
	}

	where := strings.Join(clauses, " AND ")

	query := fmt.Sprintf(`
		SELECT t.id, t.owner_id, u.username, t.title, t.status, t.last_activity_at, t.created_at
		FROM tickets t
		JOIN users u ON u.id = t.owner_id
		WHERE %s
		ORDER BY t.last_activity_at DESC
	`, where)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tickets: %w", err)
	}
	defer rows.Close()

	var tickets []Ticket
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(&t.ID, &t.OwnerID, &t.OwnerUsername, &t.Title, &t.Status, &t.LastActivityAt, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ticket: %w", err)
		}
		tickets = append(tickets, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tickets: %w", err)
	}
	return tickets, nil
}

type AttachmentWithTicket struct {
	Attachment
	TicketID int64
	OwnerID  int64
}

func (s *Store) GetAttachmentWithTicket(ctx context.Context, attachmentID int64) (*AttachmentWithTicket, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT a.id, a.message_id, a.stored_name, a.original_name, a.content_type, a.created_at,
		       m.ticket_id, t.owner_id
		FROM ticket_attachments a
		JOIN ticket_messages m ON m.id = a.message_id
		JOIN tickets t ON t.id = m.ticket_id
		WHERE a.id = $1
	`, attachmentID)

	var att AttachmentWithTicket
	var contentType *string
	if err := row.Scan(
		&att.ID,
		&att.MessageID,
		&att.StoredName,
		&att.OriginalName,
		&contentType,
		&att.CreatedAt,
		&att.TicketID,
		&att.OwnerID,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get attachment: %w", err)
	}
	att.ContentType = contentType
	return &att, nil
}
