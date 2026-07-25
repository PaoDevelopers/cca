package main

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Server struct {
	Store        *Store
	Templates    *template.Template
	Logger       *slog.Logger
	SessionMaker *SessionManager
	Attachments  *FileStorage
	InviteCode   string
	StaticDir    string
	IRCMessages  chan<- string
}

type ViewMessage struct {
	MessageWithAttachments
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	if s.StaticDir != "" {
		fs := http.FileServer(http.Dir(s.StaticDir))
		mux.Handle("/static/", http.StripPrefix("/static/", fs))
	}
	mux.HandleFunc("/", s.handleHome)
	mux.HandleFunc("/register", s.handleRegister)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/tickets", s.handleTickets)
	mux.HandleFunc("/tickets/", s.handleTicketDetail)
	mux.HandleFunc("/attachments/", s.handleAttachment)

	return s.SessionMaker.Middleware(mux)
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if user, ok := UserFromContext(r.Context()); ok && user != nil {
		http.Redirect(w, r, "/tickets", http.StatusSeeOther)
		return
	}
	s.render(w, r, "login.html", map[string]any{
		"InviteRequired": s.InviteCode != "",
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(r.FormValue("invite_code"))
	if s.InviteCode == "" || code != s.InviteCode {
		http.Error(w, "invalid invite code", http.StatusForbidden)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	if username == "" || password == "" {
		http.Error(w, "username and password required", http.StatusBadRequest)
		return
	}
	existing, err := s.Store.GetUserByUsername(r.Context(), username)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if existing != nil {
		http.Error(w, "username already exists", http.StatusConflict)
		return
	}
	hash, err := hashPassword(password)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	user, err := s.Store.CreateUser(r.Context(), username, hash, false)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.SessionMaker.CreateSession(r.Context(), w, user); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/tickets", http.StatusSeeOther)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	if username == "" || password == "" {
		http.Error(w, "username and password required", http.StatusBadRequest)
		return
	}
	user, err := s.Store.GetUserByUsername(r.Context(), username)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if user == nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	ok, err := verifyPassword(user.Password, password)
	if err != nil || !ok {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err := s.SessionMaker.CreateSession(r.Context(), w, user); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, "/tickets", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cookie, err := r.Cookie(s.SessionMaker.CookieName)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := s.SessionMaker.DestroySession(r.Context(), w, cookie.Value); err != nil {
		s.Logger.ErrorContext(r.Context(), "destroy session failed", "error", err)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleTickets(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.renderTicketList(w, r, user)
	case http.MethodPost:
		s.createTicket(w, r, user)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTicketDetail(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/tickets/")
	if idStr == "" {
		http.NotFound(w, r)
		return
	}
	segments := strings.Split(idStr, "/")
	idStr = segments[0]
	ticketID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if len(segments) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.renderTicketDetail(w, r, user, ticketID)
		return
	}

	switch segments[1] {
	case "messages":
		if r.Method == http.MethodPost {
			s.addTicketMessage(w, r, user, ticketID)
			return
		}
	case "status":
		if r.Method == http.MethodPost {
			s.updateTicketStatus(w, r, user, ticketID)
			return
		}
	case "title":
		if r.Method == http.MethodPost {
			s.updateTicketTitle(w, r, user, ticketID)
			return
		}
	}

	http.Error(w, "not found", http.StatusNotFound)
}

func (s *Server) handleAttachment(w http.ResponseWriter, r *http.Request) {
	user, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/attachments/")
	attachmentID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	meta, err := s.Store.GetAttachmentWithTicket(r.Context(), attachmentID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if meta == nil {
		http.NotFound(w, r)
		return
	}
	ticket, err := s.Store.GetTicket(r.Context(), meta.TicketID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if ticket == nil || (!user.IsAdmin && ticket.OwnerID != user.ID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	file, err := s.Attachments.Open(meta.StoredName)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			s.Logger.WarnContext(r.Context(), "closing attachment file", "error", closeErr)
		}
	}()

	if meta.ContentType != nil {
		w.Header().Set("Content-Type", *meta.ContentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", meta.OriginalName))
	if _, err := io.Copy(w, file); err != nil {
		s.Logger.ErrorContext(r.Context(), "send attachment", "error", err)
	}
}

func (s *Server) renderTicketList(w http.ResponseWriter, r *http.Request, user *SessionUser) {
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	statuses := []TicketStatus{StatusWaitingOnUser, StatusWaitingOnAdmin, StatusClosed}
	var sections []TicketSection

	for _, st := range statuses {
		items, err := s.Store.ListTicketsByStatus(r.Context(), st, user.ID, user.IsAdmin, search)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		sections = append(sections, TicketSection{
			Status:  st,
			Title:   statusTitle(st),
			Tickets: items,
		})
	}

	data := map[string]any{
		"User":              user,
		"Sections":          sections,
		"SearchQuery":       search,
		"NewTicketStatuses": statusOptions(false),
		"AllStatusOptions":  statusOptions(true),
		"InviteRestricted":  s.InviteCode != "",
		"ShowOwnerColumn":   user.IsAdmin,
	}
	s.render(w, r, "tickets.html", data)
}

func (s *Server) createTicket(w http.ResponseWriter, r *http.Request, user *SessionUser) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	body := r.FormValue("body")
	status := TicketStatus(r.FormValue("status"))
	if title == "" || strings.TrimSpace(body) == "" {
		http.Error(w, "title and message required", http.StatusBadRequest)
		return
	}
	if status != StatusWaitingOnAdmin && status != StatusWaitingOnUser {
		http.Error(w, "invalid initial status", http.StatusBadRequest)
		return
	}

	attachments, paths, err := s.saveUploadedFiles(r)
	if err != nil {
		cleanupFiles(paths, s.Logger)
		s.serverError(w, r, err)
		return
	}
	success := false
	defer func() {
		if !success {
			cleanupFiles(paths, s.Logger)
		}
	}()

	ticket, _, err := s.Store.CreateTicketWithMessage(r.Context(), user.ID, title, status, body, attachments)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.announceUserActivity(r, user, fmt.Sprintf("created ticket #%d %q", ticket.ID, ticket.Title), fmt.Sprintf("/tickets/%d", ticket.ID))
	success = true
	http.Redirect(w, r, "/tickets", http.StatusSeeOther)
}

func (s *Server) renderTicketDetail(w http.ResponseWriter, r *http.Request, user *SessionUser, ticketID int64) {
	ticket, err := s.Store.GetTicket(r.Context(), ticketID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if ticket == nil || (!user.IsAdmin && ticket.OwnerID != user.ID) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	messages, err := s.Store.GetTicketMessages(r.Context(), ticketID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	viewMessages := make([]ViewMessage, 0, len(messages))
	for _, msg := range messages {
		viewMessages = append(viewMessages, ViewMessage{
			MessageWithAttachments: msg,
		})
	}
	data := map[string]any{
		"User":              user,
		"Ticket":            ticket,
		"Messages":          viewMessages,
		"StatusOptions":     statusOptions(true),
		"TicketStatusLabel": statusTitle(ticket.Status),
		"CanRenameTicket":   user.IsAdmin,
	}
	s.render(w, r, "ticket.html", data)
}

func (s *Server) addTicketMessage(w http.ResponseWriter, r *http.Request, user *SessionUser, ticketID int64) {
	ticket, err := s.Store.GetTicket(r.Context(), ticketID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if ticket == nil || (!user.IsAdmin && ticket.OwnerID != user.ID) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	body := r.FormValue("body")
	if strings.TrimSpace(body) == "" {
		http.Error(w, "message required", http.StatusBadRequest)
		return
	}

	attachments, paths, err := s.saveUploadedFiles(r)
	if err != nil {
		cleanupFiles(paths, s.Logger)
		s.serverError(w, r, err)
		return
	}
	success := false
	defer func() {
		if !success {
			cleanupFiles(paths, s.Logger)
		}
	}()

	msg, err := s.Store.AddMessage(r.Context(), ticket.ID, user.ID, body, attachments)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.announceUserActivity(r, user, fmt.Sprintf("replied to ticket #%d %q", ticket.ID, ticket.Title), fmt.Sprintf("/tickets/%d#message-%d", ticket.ID, msg.ID))
	success = true
	http.Redirect(w, r, fmt.Sprintf("/tickets/%d", ticket.ID), http.StatusSeeOther)
}

func (s *Server) updateTicketStatus(w http.ResponseWriter, r *http.Request, user *SessionUser, ticketID int64) {
	ticket, err := s.Store.GetTicket(r.Context(), ticketID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if ticket == nil || (!user.IsAdmin && ticket.OwnerID != user.ID) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	status := TicketStatus(r.FormValue("status"))
	if err := ValidateStatus(status); err != nil {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}
	if err := s.Store.UpdateTicketStatus(r.Context(), ticket.ID, status); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.announceUserActivity(r, user, fmt.Sprintf("updated ticket #%d %q to %s", ticket.ID, ticket.Title, statusTitle(status)), fmt.Sprintf("/tickets/%d", ticket.ID))
	http.Redirect(w, r, fmt.Sprintf("/tickets/%d", ticket.ID), http.StatusSeeOther)
}

func (s *Server) updateTicketTitle(w http.ResponseWriter, r *http.Request, user *SessionUser, ticketID int64) {
	if !user.IsAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		http.Error(w, "title required", http.StatusBadRequest)
		return
	}
	if err := s.Store.UpdateTicketTitle(r.Context(), ticketID, title); err != nil {
		s.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/tickets/%d", ticketID), http.StatusSeeOther)
}

func (s *Server) saveUploadedFiles(r *http.Request) ([]AttachmentInput, []string, error) {
	form := r.MultipartForm
	if form == nil {
		return nil, nil, nil
	}
	files := form.File["attachments"]
	var inputs []AttachmentInput
	var paths []string
	for _, fh := range files {
		file, err := fh.Open()
		if err != nil {
			return nil, paths, fmt.Errorf("open upload: %w", err)
		}
		storedName, path, err := s.Attachments.Save(fh.Filename, file)
		if closeErr := file.Close(); closeErr != nil {
			s.Logger.WarnContext(r.Context(), "closing uploaded file", "error", closeErr)
		}
		if err != nil {
			if path != "" {
				paths = append(paths, path)
			}
			return nil, paths, fmt.Errorf("store attachment: %w", err)
		}
		contentType := fh.Header.Get("Content-Type")
		var ct *string
		if contentType != "" {
			ct = &contentType
		}
		inputs = append(inputs, AttachmentInput{
			StoredName:   storedName,
			OriginalName: filepath.Base(fh.Filename),
			ContentType:  ct,
		})
		paths = append(paths, path)
	}
	return inputs, paths, nil
}

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) (*SessionUser, bool) {
	user, ok := UserFromContext(r.Context())
	if !ok || user == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return nil, false
	}
	return user, true
}

func (s *Server) announceUserActivity(r *http.Request, user *SessionUser, action, path string) {
	if user == nil || user.IsAdmin {
		return
	}
	description := fmt.Sprintf("%s by %s - %s", action, user.Username, buildAbsoluteURL(r, path))
	s.sendIRC(r.Context(), description)
}

func (s *Server) sendIRC(ctx context.Context, details string) {
	if s.IRCMessages == nil {
		return
	}
	message := fmt.Sprintf("runxiyu, rx, henryxiaoyang: %s", details)
	select {
	case s.IRCMessages <- message:
	default:
		s.Logger.WarnContext(ctx, "dropping irc message", "message", message)
	}
}

func buildAbsoluteURL(r *http.Request, path string) string {
	scheme := "https"
	if r.TLS == nil {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			parts := strings.Split(proto, ",")
			if len(parts) > 0 {
				if candidate := strings.TrimSpace(parts[0]); candidate != "" {
					scheme = candidate
				}
			}
		} else {
			scheme = "http"
		}
	}
	host := strings.TrimSpace(r.Host)
	if host == "" {
		host = "localhost"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return scheme + "://" + host + path
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.Templates.ExecuteTemplate(w, name, data); err != nil {
		s.Logger.ErrorContext(r.Context(), "render template", "template", name, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	s.Logger.ErrorContext(r.Context(), "server error", "error", err)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

type TicketSection struct {
	Status  TicketStatus
	Title   string
	Tickets []Ticket
}

type StatusOption struct {
	Value string
	Label string
}

func statusTitle(status TicketStatus) string {
	switch status {
	case StatusWaitingOnAdmin:
		return "Waiting on Admin"
	case StatusWaitingOnUser:
		return "Waiting on User"
	case StatusClosed:
		return "Closed"
	default:
		return string(status)
	}
}

func statusOptions(includeClosed bool) []StatusOption {
	opts := []StatusOption{
		{Value: string(StatusWaitingOnAdmin), Label: statusTitle(StatusWaitingOnAdmin)},
		{Value: string(StatusWaitingOnUser), Label: statusTitle(StatusWaitingOnUser)},
	}
	if includeClosed {
		opts = append(opts, StatusOption{Value: string(StatusClosed), Label: statusTitle(StatusClosed)})
	}
	return opts
}

func cleanupFiles(paths []string, logger *slog.Logger) {
	for _, path := range paths {
		if path == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			logger.Warn("attachment cleanup failed", "path", path, "error", err)
		}
	}
}
