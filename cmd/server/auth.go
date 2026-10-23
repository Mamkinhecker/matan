package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"crypto/pbkdf2"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	sessionCookieName  = "matan_session"
	sessionDuration    = 30 * 24 * time.Hour
	passwordIterations = 600_000
	csrfCookieName     = "matan_csrf"
)

type authContextKey struct{}

type authUser struct {
	ID          int64
	DisplayName string
	Username    string
	IsAdmin     bool
}

func (a *App) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil && cookie.Value != "" {
			tokenHash := sessionTokenHash(cookie.Value)
			var user authUser
			err := a.db.QueryRow(r.Context(), `
				select u.id, u.display_name, u.username, u.is_admin
				from sessions s
				join users u on u.id = s.user_id
				where s.token_hash = $1 and s.expires_at > now()
			`, tokenHash).Scan(&user.ID, &user.DisplayName, &user.Username, &user.IsAdmin)
			switch {
			case err == nil:
				r = r.WithContext(context.WithValue(r.Context(), authContextKey{}, user))
			case errors.Is(err, pgx.ErrNoRows):
				clearSessionCookie(w, r)
			default:
				log.Printf("load session: %v", err)
				http.Error(w, "Не удалось проверить сессию.", http.StatusInternalServerError)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := currentUser(r)
		if !ok {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}
		if !user.IsAdmin {
			http.Error(w, "Доступ к админке запрещён.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func createAdminAccount(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, username, displayName string) error {
	username = strings.ToLower(strings.TrimSpace(username))
	displayName = strings.TrimSpace(displayName)
	if !validUsername(username) {
		return errors.New("username must be 3-32 lowercase latin letters, digits, _ or -")
	}
	if !validDisplayName(displayName) {
		return errors.New("display name must be 2-40 characters")
	}

	passwordBytes := make([]byte, 24)
	if _, err := rand.Read(passwordBytes); err != nil {
		return fmt.Errorf("generate password: %w", err)
	}
	password := base64.RawURLEncoding.EncodeToString(passwordBytes)
	passwordHash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	var userID int64
	err = db.QueryRow(ctx, `
		insert into users (username, display_name, password_hash, is_admin)
		values ($1, $2, $3, true)
		returning id
	`, username, displayName, passwordHash).Scan(&userID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("username %q already exists", username)
		}
		return fmt.Errorf("insert admin account: %w", err)
	}

	fmt.Printf("Admin account created.\nUsername: %s\nDisplay name: %s\nPassword: %s\n", username, displayName, password)
	return nil
}

func (a *App) requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Некорректная форма.", http.StatusBadRequest)
			return
		}
		if !validCSRF(r) {
			http.Error(w, "Форма устарела. Обновите страницу и попробуйте ещё раз.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func currentUser(r *http.Request) (authUser, bool) {
	user, ok := r.Context().Value(authContextKey{}).(authUser)
	return user, ok
}

func (a *App) loginPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := currentUser(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	a.authPage(w, r, "login", r.URL.Query().Get("message"), safeRedirectPath(r.URL.Query().Get("next"), "/"))
}

func (a *App) registerPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := currentUser(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	a.authPage(w, r, "register", r.URL.Query().Get("message"), safeRedirectPath(r.URL.Query().Get("next"), "/"))
}

func (a *App) authPage(w http.ResponseWriter, r *http.Request, mode, message, next string) {
	title := "Вход"
	if mode == "register" {
		title = "Регистрация"
	}
	a.render(w, r, "auth.html", PageData{
		Title:    title,
		Active:   "auth",
		Message:  message,
		AuthMode: mode,
		Next:     next,
	})
}

func (a *App) register(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Некорректная форма.", http.StatusBadRequest)
		return
	}
	if !validCSRF(r) {
		http.Error(w, "Форма устарела. Обновите страницу и попробуйте ещё раз.", http.StatusForbidden)
		return
	}
	displayName := strings.TrimSpace(r.FormValue("display_name"))
	username := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	password := r.FormValue("password")
	next := safeRedirectPath(r.FormValue("next"), "/")

	if !validUsername(username) {
		a.authPage(w, r, "register", "Логин: 3–32 символа, только латинские буквы, цифры, _ и -.", next)
		return
	}
	if !validDisplayName(displayName) {
		a.authPage(w, r, "register", "Имя должно содержать от 2 до 40 символов.", next)
		return
	}
	if utf8.RuneCountInString(password) < 8 || utf8.RuneCountInString(password) > 128 {
		a.authPage(w, r, "register", "Пароль должен содержать от 8 до 128 символов.", next)
		return
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		log.Printf("hash password: %v", err)
		http.Error(w, "Не удалось создать аккаунт.", http.StatusInternalServerError)
		return
	}
	var userID int64
	err = a.db.QueryRow(r.Context(), `
		insert into users (username, display_name, password_hash)
		values ($1, $2, $3)
		returning id
	`, username, displayName, passwordHash).Scan(&userID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			a.authPage(w, r, "register", "Такой логин уже занят.", next)
			return
		}
		log.Printf("register user: %v", err)
		http.Error(w, "Не удалось создать аккаунт.", http.StatusInternalServerError)
		return
	}
	if err := a.createSession(w, r, userID); err != nil {
		log.Printf("create session after registration: %v", err)
		http.Error(w, "Аккаунт создан, но войти не удалось. Попробуйте войти.", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Некорректная форма.", http.StatusBadRequest)
		return
	}
	if !validCSRF(r) {
		http.Error(w, "Форма устарела. Обновите страницу и попробуйте ещё раз.", http.StatusForbidden)
		return
	}
	username := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	password := r.FormValue("password")
	next := safeRedirectPath(r.FormValue("next"), "/")

	var user authUser
	var passwordHash string
	err := a.db.QueryRow(r.Context(), `
		select id, display_name, password_hash
		from users
		where username = $1
	`, username).Scan(&user.ID, &user.DisplayName, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !verifyPassword(password, passwordHash)) {
		a.authPage(w, r, "login", "Неверный логин или пароль.", next)
		return
	}
	if err != nil {
		log.Printf("login user: %v", err)
		http.Error(w, "Не удалось войти.", http.StatusInternalServerError)
		return
	}
	if err := a.createSession(w, r, user.ID); err != nil {
		log.Printf("create session after login: %v", err)
		http.Error(w, "Не удалось войти.", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Некорректная форма.", http.StatusBadRequest)
		return
	}
	if !validCSRF(r) {
		http.Error(w, "Форма устарела. Обновите страницу и попробуйте ещё раз.", http.StatusForbidden)
		return
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		if _, err := a.db.Exec(r.Context(), "delete from sessions where token_hash = $1", sessionTokenHash(cookie.Value)); err != nil {
			log.Printf("delete session: %v", err)
			http.Error(w, "Не удалось завершить сессию.", http.StatusInternalServerError)
			return
		}
	}
	clearSessionCookie(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) createSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return fmt.Errorf("generate session token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)
	expiresAt := time.Now().Add(sessionDuration)
	if _, err := a.db.Exec(r.Context(), "delete from sessions where expires_at <= now()"); err != nil {
		return fmt.Errorf("remove expired sessions: %w", err)
	}
	if _, err := a.db.Exec(r.Context(), `
		insert into sessions (token_hash, user_id, expires_at)
		values ($1, $2, $3)
	`, sessionTokenHash(token), userID, expiresAt); err != nil {
		return fmt.Errorf("store session: %w", err)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
		SameSite: http.SameSiteLaxMode,
	})
}

func csrfToken(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(csrfCookieName); err == nil {
		if tokenBytes, err := base64.RawURLEncoding.DecodeString(cookie.Value); err == nil && len(tokenBytes) == 32 {
			return cookie.Value
		}
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		log.Printf("generate CSRF token: %v", err)
		return ""
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
		SameSite: http.SameSiteLaxMode,
	})
	return token
}

func validCSRF(r *http.Request) bool {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	formToken := r.FormValue("csrf_token")
	return len(formToken) == len(cookie.Value) && subtle.ConstantTimeCompare([]byte(formToken), []byte(cookie.Value)) == 1
}

func sessionTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, passwordIterations, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", passwordIterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func verifyPassword(password, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 1 || iterations > 2_000_000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) != 16 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(expected) != 32 {
		return false
	}
	actual, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(expected))
	return err == nil && subtle.ConstantTimeCompare(actual, expected) == 1
}

func validUsername(username string) bool {
	if len(username) < 3 || len(username) > 32 {
		return false
	}
	for _, char := range username {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func validDisplayName(name string) bool {
	length := utf8.RuneCountInString(name)
	if length < 2 || length > 40 {
		return false
	}
	for _, char := range name {
		if unicode.IsControl(char) {
			return false
		}
	}
	return true
}

func safeRedirectPath(value, fallback string) string {
	if value == "" {
		return fallback
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") || strings.HasPrefix(parsed.Path, "//") || strings.Contains(parsed.Path, "\\") {
		return fallback
	}
	return parsed.RequestURI()
}

func initials(name string) string {
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return "?"
	}
	first, _ := utf8.DecodeRuneInString(fields[0])
	return strings.ToUpper(string(first))
}

func templateFunctions() template.FuncMap {
	return template.FuncMap{
		"add":      func(a, b int) int { return a + b },
		"mul":      func(a, b int) int { return a * b },
		"initials": initials,
	}
}
