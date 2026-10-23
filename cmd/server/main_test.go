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

func TestValidateAnswerRejectsEmptyAnswer(t *testing.T) {
	grade, feedback := validateAnswer(" \n\t", "предел + значение", "Полный ответ")
	if grade != "bad" || feedback == "" {
		t.Fatalf("expected empty answer to be rejected with feedback, got grade=%q feedback=%q", grade, feedback)
	}
}

func TestParseIssuedExamIDs(t *testing.T) {
	tests := []struct {
		name      string
		submitted []string
		issued    []int64
		wantErr   bool
	}{
		{name: "same set", submitted: []string{"7", "8"}, issued: []int64{8, 7}},
		{name: "duplicate", submitted: []string{"7", "7"}, issued: []int64{7, 8}, wantErr: true},
		{name: "unissued id", submitted: []string{"7", "9"}, issued: []int64{7, 8}, wantErr: true},
		{name: "missing id", submitted: []string{"7"}, issued: []int64{7, 8}, wantErr: true},
		{name: "invalid id", submitted: []string{"0"}, issued: []int64{7}, wantErr: true},
		{name: "duplicate issued id", submitted: []string{"7", "8"}, issued: []int64{7, 7}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseIssuedExamIDs(test.submitted, test.issued)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseIssuedExamIDs() error = %v, wantErr %v", err, test.wantErr)
			}
			if err == nil && len(got) != len(test.submitted) {
				t.Fatalf("got %d parsed ids, want %d", len(got), len(test.submitted))
			}
		})
	}
}

func TestValidExamRunID(t *testing.T) {
	if !validExamRunID(strings.Repeat("a1", 32)) {
		t.Fatal("valid 32-byte hex exam run ID rejected")
	}
	for _, id := range []string{"", strings.Repeat("g", 64), strings.Repeat("a", 63)} {
		if validExamRunID(id) {
			t.Errorf("invalid exam run ID accepted: %q", id)
		}
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

func TestPracticeAndAnonymousExamTemplatesRender(t *testing.T) {
	tests := []struct {
		name     string
		data     PageData
		contains string
	}{
		{
			name:     "practice.html",
			data:     PageData{Title: "Настроить тренировку"},
			contains: "Сформировать вариант",
		},
		{
			name: "variant.html",
			data: PageData{
				Title: "Экзамен", VariantTitle: "Экзамен", IsExam: true, ExamMinutes: 45,
				Questions: []Question{{ID: 1, Text: "Задание"}},
			},
			contains: "попытка сохранится в прогрессе только после авторизации",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest("GET", "/", nil)
			(&App{}).render(recorder, request, test.name, test.data)
			if recorder.Code != http.StatusOK {
				t.Fatalf("render status = %d: %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), test.contains) {
				t.Errorf("rendered page does not contain %q", test.contains)
			}
		})
	}
}

func TestHomeUsesNewBrandWithoutIntroductoryProjectCopy(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/", nil)
	(&App{}).render(recorder, request, "home.html", PageData{
		Title: "Matan FAMCS",
		Stats: map[string]int{},
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("render status = %d: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, expected := range []string{"Matan FAMCS", "Настроить тренировку", "Повторить сложное", "design-new"} {
		if !strings.Contains(body, expected) {
			t.Errorf("home page does not contain %q", expected)
		}
	}
	for _, removed := range []string{"PostgreSQL · личный прогресс и рейтинг", "Тренажер под реальные письменные варианты"} {
		if strings.Contains(body, removed) {
			t.Errorf("home page still contains removed project intro %q", removed)
		}
	}
}

func TestAnonymousNavigationStartsWithAuthLinksAndThemeToggle(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/", nil)
	(&App{}).render(recorder, request, "home.html", PageData{Title: "Matan FAMCS"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("render status = %d: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	login := strings.Index(body, `href="/login">Войти</a>`)
	register := strings.Index(body, `href="/register">Регистрация</a>`)
	topics := strings.Index(body, `href="/topics">Темы</a>`)
	if login < 0 || register < 0 || topics < 0 || login > register || register > topics {
		t.Fatalf("anonymous navigation does not start with login and registration links: login=%d register=%d topics=%d", login, register, topics)
	}
	if !strings.Contains(body, `data-theme-toggle`) {
		t.Error("header is missing the theme toggle")
	}
	if navStart, themeToggle := strings.Index(body, `<nav class="nav"`), strings.Index(body, `data-theme-toggle`); navStart < 0 || themeToggle > navStart {
		t.Error("theme toggle should sit outside and before the primary navigation")
	}
}

func TestCuratedExtraTasksHaveCompleteSolutions(t *testing.T) {
	if len(curatedExtraTasks) != 30 {
		t.Fatalf("curated extra task count = %d, want 30", len(curatedExtraTasks))
	}
	seen := make(map[string]bool, len(curatedExtraTasks))
	topics := make(map[string]map[string]int)
	for _, task := range curatedExtraTasks {
		if task.topic == "" || task.text == "" || task.answer == "" || task.short == "" {
			t.Errorf("incomplete task data: %#v", task)
		}
		if task.kind != "ikr" && task.kind != "tkr" {
			t.Errorf("unexpected task kind %q", task.kind)
		}
		wantType := map[string]string{"ikr": "explain", "tkr": "prove"}[task.kind]
		if got := curatedQuestionType(task.kind); got != wantType {
			t.Errorf("question type for kind %q = %q, want %q", task.kind, got, wantType)
		}
		if topics[task.topic] == nil {
			topics[task.topic] = make(map[string]int)
		}
		topics[task.topic][task.kind]++
		if seen[task.text] {
			t.Errorf("duplicate task text %q", task.text)
		}
		seen[task.text] = true
		if len([]rune(task.answer)) < 200 {
			t.Errorf("solution for %q is too short to be a complete derivation", task.text)
		}
		if !strings.Contains(task.answer, ".") {
			t.Errorf("solution for %q does not contain a complete sentence", task.text)
		}
	}
	if len(topics) != 15 {
		t.Errorf("extra tasks cover %d topics, want 15", len(topics))
	}
	for topic, kinds := range topics {
		if kinds["ikr"] != 1 || kinds["tkr"] != 1 {
			t.Errorf("topic %q has IKR/TKR counts %d/%d, want 1/1", topic, kinds["ikr"], kinds["tkr"])
		}
	}
	if curatedQuestionType("invalid") != "" {
		t.Error("unsupported task kind must not map to a database question type")
	}
}

func TestPracticeParameterHelpers(t *testing.T) {
	if got := validPracticeKind("ikr"); got != "ikr" {
		t.Errorf("validPracticeKind(ikr) = %q", got)
	}
	if got := validPracticeKind("arbitrary"); got != "" {
		t.Errorf("validPracticeKind(arbitrary) = %q, want empty", got)
	}
	if got := clampInt(99, 3, 20); got != 20 {
		t.Errorf("clampInt high = %d, want 20", got)
	}
	if got := clampInt(1, 3, 20); got != 3 {
		t.Errorf("clampInt low = %d, want 3", got)
	}
	if got := buildPracticeURL(2, "ikr", 5, 4, 10, true, 60); got != "/variant?count=10&difficulty=4&exam=1&kind=ikr&minutes=60&semester=2&topic=5" {
		t.Errorf("buildPracticeURL = %q", got)
	}
}
