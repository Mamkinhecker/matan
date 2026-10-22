package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGradePoints(t *testing.T) {
	tests := map[string]int{"perfect": 4, "good": 3, "almost": 1, "bad": 0, "unknown": 0}
	for grade, want := range tests {
		if got := gradePoints(grade); got != want {
			t.Errorf("gradePoints(%q)=%d, want %d", grade, got, want)
		}
	}
}

func TestValidateAnswerRequiresMeaningfulElements(t *testing.T) {
	grade, feedback := validateAnswer("Нужно применить критерий Коши и проверить условия сходимости последовательности. Затем сделать итоговый вывод.", "критерий Коши + условия + вывод", "Полный ответ")
	if grade != "good" && grade != "perfect" {
		t.Fatalf("grade=%q, feedback=%q", grade, feedback)
	}
	if feedback == "" {
		t.Fatal("validator returned empty feedback")
	}
}

func TestValidateAnswerGivesNextStep(t *testing.T) {
	grade, feedback := validateAnswer("Я применю признак.", "условия + сравнение + вывод", "Полный ответ")
	if grade == "perfect" || feedback == "" {
		t.Fatalf("expected constructive feedback, got grade=%q feedback=%q", grade, feedback)
	}
}

func TestValidUsername(t *testing.T) {
	for _, username := range []string{"alex", "student_7", "user-name"} {
		if !validUsername(username) {
			t.Errorf("validUsername(%q) = false", username)
		}
	}
	for _, username := range []string{"ab", "too-long-username-with-over-32-chars", "имя", "bad name", "user@example.com"} {
		if validUsername(username) {
			t.Errorf("validUsername(%q) = true", username)
		}
	}
}

func TestValidDisplayName(t *testing.T) {
	for _, name := range []string{"Анна", "Alex Morgan"} {
		if !validDisplayName(name) {
			t.Errorf("validDisplayName(%q) = false", name)
		}
	}
	for _, name := range []string{"A", strings.Repeat("я", 41), "Name\nInjected"} {
		if validDisplayName(name) {
			t.Errorf("validDisplayName(%q) = true", name)
		}
	}
}

func TestSafeRedirectPath(t *testing.T) {
	for _, value := range []string{"https://evil.example", "//evil.example", "/\\evil.example"} {
		if got := safeRedirectPath(value, "/"); got != "/" {
			t.Errorf("safeRedirectPath(%q) = %q, want /", value, got)
		}
	}
	if got := safeRedirectPath("/progress?week=1", "/"); got != "/progress?week=1" {
		t.Errorf("safeRedirectPath returned %q", got)
	}
}

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hashPassword: %v", err)
	}
	if !verifyPassword("correct horse battery staple", hash) {
		t.Fatal("verifyPassword rejected the correct password")
	}
	if verifyPassword("wrong password", hash) {
		t.Fatal("verifyPassword accepted the wrong password")
	}
}

func TestValidCSRF(t *testing.T) {
	request := httptest.NewRequest("POST", "/login", strings.NewReader("csrf_token=abc123"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "abc123"})
	if err := request.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	if !validCSRF(request) {
		t.Fatal("validCSRF rejected a matching token")
	}
	request.Form.Set("csrf_token", "different")
	if validCSRF(request) {
		t.Fatal("validCSRF accepted a mismatched token")
	}
}

func TestInitials(t *testing.T) {
	if got := initials("Анна Петрова"); got != "А" {
		t.Errorf("initials() = %q, want А", got)
	}
}

func TestAdminAuthorizationMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		user       *authUser
		wantStatus int
		wantCalled bool
	}{
		{name: "anonymous", wantStatus: http.StatusSeeOther},
		{name: "regular user", user: &authUser{ID: 2}, wantStatus: http.StatusForbidden},
		{name: "admin", user: &authUser{ID: 3, IsAdmin: true}, wantStatus: http.StatusOK, wantCalled: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			called := false
			handler := (&App{}).adminOnly(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				called = true
			}))
			request := httptest.NewRequest("GET", "/admin", nil)
			if test.user != nil {
				request = request.WithContext(context.WithValue(request.Context(), authContextKey{}, *test.user))
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			if called != test.wantCalled {
				t.Errorf("handler called = %v, want %v", called, test.wantCalled)
			}
		})
	}
}

func TestRequireCSRFRejectsInvalidToken(t *testing.T) {
	called := false
	handler := (&App{}).requireCSRF(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	request := httptest.NewRequest("POST", "/admin/topics", strings.NewReader("csrf_token=wrong"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "expected"})
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if called {
		t.Error("handler ran despite invalid CSRF token")
	}
}

func TestAdminFormsHaveCSRFFields(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/admin", nil)
	(&App{}).render(recorder, request, "admin.html", PageData{
		Title:     "Админка",
		IsAdmin:   true,
		CSRFToken: "test-token",
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("render status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := strings.Count(recorder.Body.String(), `name="csrf_token"`); got != 5 {
		t.Errorf("admin page has %d CSRF fields, want 5", got)
	}
}
