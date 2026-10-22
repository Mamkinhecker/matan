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

func TestVariantFormsHaveCSRFFields(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/ikr", nil)
	(&App{}).render(recorder, request, "variant.html", PageData{
		Title:        "ИКР",
		VariantTitle: "Вариант ИКР",
		Questions: []Question{{
			ID:             1,
			ExamKind:       "ikr",
			Text:           "Задание",
			ExpectedAnswer: "Решение",
		}},
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("render status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := strings.Count(recorder.Body.String(), `name="csrf_token"`); got != 2 {
		t.Errorf("variant page has %d CSRF fields, want 2", got)
	}
	if strings.Contains(recorder.Body.String(), `value="">`) {
		t.Error("variant form has an empty CSRF token")
	}
}

func TestTopicSearchResultsRenderMatchedTheoryTitles(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/topics?q=Коши", nil)
	(&App{}).render(recorder, request, "topics.html", PageData{
		Title:       "Темы",
		SearchQuery: "Коши",
		Topics: []Topic{{
			ID:           1,
			Title:        "Последовательности",
			Semester:     1,
			MatchedItems: []string{"Критерий Коши"},
		}},
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("render status = %d: %s", recorder.Code, recorder.Body.String())
	}
	for _, expected := range []string{"Коши", "Критерий Коши", "Последовательности"} {
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Errorf("topic search page does not contain %q", expected)
		}
	}
}

func TestGroupCardsBySemesterAndTopic(t *testing.T) {
	cards := []Card{
		{ID: 1, TopicID: 10, Semester: 1, TopicTitle: "Последовательности"},
		{ID: 2, TopicID: 10, Semester: 1, TopicTitle: "Последовательности"},
		{ID: 3, TopicID: 20, Semester: 1, TopicTitle: "Ряды"},
		{ID: 4, TopicID: 30, Semester: 2, TopicTitle: "Интегралы"},
	}

	groups := groupCards(cards)
	if len(groups) != 2 {
		t.Fatalf("got %d semester groups, want 2", len(groups))
	}
	if groups[0].Title != "1 семестр" || len(groups[0].Topics) != 2 {
		t.Fatalf("unexpected first semester grouping: %#v", groups[0])
	}
	if groups[0].Topics[0].TopicTitle != "Последовательности" || len(groups[0].Topics[0].Cards) != 2 {
		t.Fatalf("cards not grouped by topic: %#v", groups[0].Topics[0])
	}
	if groups[1].Title != "2 семестр" || groups[1].Topics[0].TopicTitle != "Интегралы" {
		t.Fatalf("unexpected second semester grouping: %#v", groups[1])
	}
}

func TestCardsPageRendersGroupedCardsWithCSRFToken(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/cards", nil)
	(&App{}).render(recorder, request, "cards.html", PageData{
		Title:      "Карточки",
		CardGroups: groupCards([]Card{{ID: 1, TopicID: 10, Semester: 1, TopicTitle: "Последовательности", Front: "Определение", Back: "Ответ"}}),
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("render status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "1 семестр") || !strings.Contains(recorder.Body.String(), "Последовательности") {
		t.Error("cards page is missing semester or topic grouping")
	}
	if got := strings.Count(recorder.Body.String(), `name="csrf_token"`); got != 1 {
		t.Errorf("cards page has %d CSRF fields, want 1", got)
	}
}
