package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed templates/*.html static/*
var assets embed.FS

type App struct {
	db *pgxpool.Pool
}

type Topic struct {
	ID          int
	Title       string
	Chapter     string
	Description string
	Difficulty  int
	Semester    int
	SortOrder   int
	Items       []TheoryItem
}

type TheoryItem struct {
	ID           int
	TopicID      int
	Type         string
	Title        string
	Statement    string
	Explanation  string
	ProofOutline string
	ProofFull    string
	SourcePage   int
}

type Card struct {
	ID         int
	TopicID    int
	Type       string
	Front      string
	Back       string
	Hint       string
	Difficulty int
	TopicTitle string
}

type Question struct {
	ID             int
	TopicID        int
	TheoryItemID   int
	Type           string
	ExamKind       string
	Text           string
	ExpectedAnswer string
	ShortAnswer    string
	Difficulty     int
	Tags           []string
	TopicTitle     string
	StudentAnswer  string
	Feedback       string
}

type ProofChoice struct {
	ID             int
	TheoryItemID   int
	Question       string
	CorrectOutline string
	Wrong1         string
	Wrong2         string
	Wrong3         string
	Explanation    string
	TopicTitle     string
	TheoryTitle    string
	Options        []ProofOption
}

type ProofOption struct {
	Text    string
	Correct bool
}

type ProgressRow struct {
	Mode          string
	Grade         string
	Attempts      int
	LastAttemptAt string
}

type LeaderRow struct {
	Name     string
	Attempts int
	Points   int
	Average  float64
}

type PageData struct {
	Title        string
	Active       string
	Message      string
	Stats        map[string]int
	Topics       []Topic
	Items        []TheoryItem
	Topic        Topic
	Cards        []Card
	Card         Card
	Questions    []Question
	Question     Question
	Game         ProofChoice
	Progress     []ProgressRow
	Leaderboard  []LeaderRow
	VariantTitle string
	CurrentUser  string
	IsAdmin      bool
	AuthMode     string
	Next         string
	CSRFToken    string
}

func main() {
	ctx := context.Background()
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		if os.Getenv("RENDER") == "true" {
			log.Fatal("DATABASE_URL is required on Render; add the Supabase Session pooler URI in the service Environment settings")
		}
		databaseURL = "postgres://postgres:postgres@localhost:5544/matan_trainer?sslmode=disable"
	}

	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("connect database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		log.Fatalf("ping database: %v", err)
	}
	if err := migrate(ctx, db); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if len(os.Args) > 1 {
		if os.Args[1] != "create-admin" || len(os.Args) != 4 {
			log.Fatal("usage: go run ./cmd/server create-admin <username> <display name>")
		}
		if err := createAdminAccount(ctx, db, os.Args[2], strings.Join(os.Args[3:], " ")); err != nil {
			log.Fatalf("create admin: %v", err)
		}
		return
	}
	if err := seed(ctx, db); err != nil {
		log.Fatalf("seed: %v", err)
	}

	app := &App{db: db}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.FileServerFS(assets))
	mux.HandleFunc("GET /login", app.loginPage)
	mux.HandleFunc("POST /login", app.login)
	mux.HandleFunc("GET /register", app.registerPage)
	mux.HandleFunc("POST /register", app.register)
	mux.HandleFunc("POST /logout", app.logout)
	mux.HandleFunc("GET /", app.home)
	mux.HandleFunc("GET /topics", app.topics)
	mux.HandleFunc("GET /topic/", app.topic)
	mux.HandleFunc("GET /cards", app.cards)
	mux.HandleFunc("GET /formulations", app.formulations)
	mux.HandleFunc("GET /proof-game", app.proofGame)
	mux.HandleFunc("GET /oral", app.oral)
	mux.HandleFunc("GET /variant", app.variant)
	mux.HandleFunc("GET /theory-test", app.theoryTest)
	mux.HandleFunc("GET /ikr", app.ikr)
	mux.HandleFunc("GET /tkr", app.tkr)
	mux.HandleFunc("GET /progress", app.progress)
	mux.HandleFunc("GET /leaderboard", app.leaderboard)
	mux.Handle("GET /admin", app.adminOnly(http.HandlerFunc(app.admin)))
	mux.HandleFunc("POST /attempts", app.recordAttempt)
	mux.Handle("POST /admin/topics", app.adminOnly(app.requireCSRF(http.HandlerFunc(app.createTopic))))
	mux.Handle("POST /admin/theory", app.adminOnly(app.requireCSRF(http.HandlerFunc(app.createTheory))))
	mux.Handle("POST /admin/cards", app.adminOnly(app.requireCSRF(http.HandlerFunc(app.createCard))))
	mux.Handle("POST /admin/questions", app.adminOnly(app.requireCSRF(http.HandlerFunc(app.createQuestion))))
	mux.Handle("POST /admin/proof-games", app.adminOnly(app.requireCSRF(http.HandlerFunc(app.createProofGame))))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}
	addr := port
	log.Printf("matan trainer listening on http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, app.authenticate(mux)))
}

func (a *App) render(w http.ResponseWriter, r *http.Request, name string, data PageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if user, ok := currentUser(r); ok {
		data.CurrentUser = user.DisplayName
		data.IsAdmin = user.IsAdmin
	}
	data.CSRFToken = csrfToken(w, r)
	if data.CSRFToken == "" {
		http.Error(w, "Не удалось подготовить защищённую форму.", http.StatusInternalServerError)
		return
	}
	tmpl, err := template.New(name).Funcs(templateFunctions()).ParseFS(assets, "templates/layout.html", "templates/"+name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	stats, err := a.stats(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, r, "home.html", PageData{Title: "Matan Trainer", Active: "home", Stats: stats})
}

func (a *App) topics(w http.ResponseWriter, r *http.Request) {
	topics, err := a.listTopics(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, r, "topics.html", PageData{Title: "Темы", Active: "topics", Topics: topics})
}

func (a *App) topic(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/topic/"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	topic, err := a.getTopic(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a.render(w, r, "topic.html", PageData{Title: topic.Title, Active: "topics", Topic: topic})
}

func (a *App) cards(w http.ResponseWriter, r *http.Request) {
	cards, err := a.listCards(r.Context(), r.URL.Query().Get("topic"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	topics, _ := a.listTopics(r.Context())
	a.render(w, r, "cards.html", PageData{Title: "Карточки", Active: "cards", Cards: cards, Topics: topics})
}

func (a *App) formulations(w http.ResponseWriter, r *http.Request) {
	card, err := a.randomCard(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, r, "formulations.html", PageData{Title: "Формулировки", Active: "formulations", Card: card})
}

func (a *App) proofGame(w http.ResponseWriter, r *http.Request) {
	game, err := a.randomProofGame(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, r, "proof_game.html", PageData{Title: "Доказательства", Active: "proof-game", Game: game})
}

func (a *App) oral(w http.ResponseWriter, r *http.Request) {
	question, err := a.randomQuestion(r.Context(), "oral")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, r, "oral.html", PageData{Title: "Устный тест", Active: "oral", Question: question})
}

func (a *App) variant(w http.ResponseWriter, r *http.Request) {
	semester := semesterParam(r)
	questions, err := a.variantQuestions(r.Context(), "", 6, semester)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, r, "variant.html", PageData{Title: "Письменный вариант", Active: "variant", Questions: questions, VariantTitle: "Смешанный вариант · " + semesterTitle(semester)})
}

func (a *App) theoryTest(w http.ResponseWriter, r *http.Request) {
	semester := semesterParam(r)
	questions, err := a.variantQuestions(r.Context(), "theory", 8, semester)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, r, "variant.html", PageData{Title: "Тест по теории", Active: "theory-test", Questions: questions, VariantTitle: "Тест по теории · " + semesterTitle(semester)})
}

func (a *App) ikr(w http.ResponseWriter, r *http.Request) {
	semester := semesterParam(r)
	questions, err := a.variantQuestions(r.Context(), "ikr", 6, semester)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, r, "variant.html", PageData{Title: "ИКР", Active: "ikr", Questions: questions, VariantTitle: "Вариант ИКР · " + semesterTitle(semester)})
}

func (a *App) tkr(w http.ResponseWriter, r *http.Request) {
	semester := semesterParam(r)
	questions, err := a.variantQuestions(r.Context(), "tkr", 6, semester)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, r, "variant.html", PageData{Title: "ТКР", Active: "tkr", Questions: questions, VariantTitle: "Вариант ТКР · " + semesterTitle(semester)})
}

func (a *App) progress(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		http.Redirect(w, r, "/login?next=%2Fprogress", http.StatusSeeOther)
		return
	}
	progress, err := a.progressRows(r.Context(), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, r, "progress.html", PageData{Title: "Прогресс", Active: "progress", Progress: progress})
}

func (a *App) admin(w http.ResponseWriter, r *http.Request) {
	topics, err := a.listTopics(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items, err := a.listTheoryItems(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	stats, err := a.stats(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, r, "admin.html", PageData{
		Title:   "Админка",
		Active:  "admin",
		Message: r.URL.Query().Get("message"),
		Topics:  topics,
		Items:   items,
		Stats:   stats,
	})
}

func (a *App) recordAttempt(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !validCSRF(r) {
		http.Error(w, "Форма устарела. Обновите страницу и попробуйте ещё раз.", http.StatusForbidden)
		return
	}
	user, ok := currentUser(r)
	if !ok {
		next := safeRedirectPath(r.FormValue("redirect"), "/")
		http.Redirect(w, r, "/login?next="+url.QueryEscape(next), http.StatusSeeOther)
		return
	}
	targetID, _ := strconv.Atoi(r.FormValue("target_id"))
	grade := r.FormValue("grade")
	studentAnswer := strings.TrimSpace(r.FormValue("student_answer"))
	feedback := ""
	if targetID > 0 && studentAnswer != "" && r.FormValue("target_type") == "question" {
		var expected, short string
		if err := a.db.QueryRow(r.Context(), "select expected_answer, short_answer from questions where id = $1", targetID).Scan(&expected, &short); err == nil {
			grade, feedback = validateAnswer(studentAnswer, short, expected)
		}
	}
	_, err := a.db.Exec(r.Context(), `
		insert into attempts (demo_student_id, target_type, target_id, mode, grade, notes, student_answer, feedback, points)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, user.ID, r.FormValue("target_type"), targetID, r.FormValue("mode"), grade, r.FormValue("notes"), studentAnswer, feedback, gradePoints(grade))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, safeRedirectPath(r.FormValue("redirect"), "/"), http.StatusSeeOther)
}

func (a *App) leaderboard(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(r.Context(), `
		select u.display_name, count(*), coalesce(sum(a.points), 0), coalesce(avg(a.points), 0)
		from attempts a
		join users u on u.id = a.demo_student_id
		group by u.id, u.display_name
		order by sum(a.points) desc, count(*) desc, u.display_name
		limit 50
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var leaders []LeaderRow
	for rows.Next() {
		var row LeaderRow
		if err := rows.Scan(&row.Name, &row.Attempts, &row.Points, &row.Average); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		leaders = append(leaders, row)
	}
	a.render(w, r, "leaderboard.html", PageData{Title: "Топ", Active: "leaderboard", Leaderboard: leaders})
}

func gradePoints(grade string) int {
	switch grade {
	case "perfect":
		return 4
	case "good":
		return 3
	case "almost":
		return 1
	default:
		return 0
	}
}

func validateAnswer(answer, short, expected string) (string, string) {
	text := strings.ToLower(answer)
	clean := strings.NewReplacer("+", " ", ",", " ", ".", " ", ":", " ", ";", " ", "(", " ", ")", " ").Replace(strings.ToLower(short))
	stop := map[string]bool{"и": true, "или": true, "для": true, "при": true, "через": true, "это": true, "нужно": true, "если": true, "из": true, "по": true}
	missing := make([]string, 0, 4)
	seen := map[string]bool{}
	for _, word := range strings.Fields(clean) {
		if len([]rune(word)) < 4 || stop[word] || seen[word] {
			continue
		}
		seen[word] = true
		if !strings.Contains(text, word) {
			missing = append(missing, word)
		}
	}
	if len(missing) == 0 && len([]rune(answer)) >= 80 {
		return "perfect", "Ответ содержит все обязательные смысловые элементы и достаточную аргументацию."
	}
	if len(missing) <= 2 && len([]rune(answer)) >= 45 {
		return "good", "Основная структура есть. В следующий раз добавь: " + strings.Join(missing, ", ") + "."
	}
	if len(missing) > 0 {
		return "almost", "В следующий раз добавь обязательные элементы: " + strings.Join(missing, ", ") + ". Эталон: " + expected
	}
	return "almost", "В следующий раз распиши переходы доказательства подробнее и добавь условия применимости."
}

func (a *App) createTopic(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := a.db.Exec(r.Context(), `
		insert into topics (title, chapter, description, difficulty)
		values ($1, $2, $3, $4)
	`, required(r, "title"), r.FormValue("chapter"), r.FormValue("description"), formInt(r, "difficulty", 1))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	redirect(w, r, "/admin?message=topic")
}

func (a *App) createTheory(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := a.db.Exec(r.Context(), `
		insert into theory_items (topic_id, type, title, statement, explanation, proof_outline, proof_full, source_page)
		values ($1, $2, $3, $4, $5, $6, $7, nullif($8, 0))
	`, formInt(r, "topic_id", 0), required(r, "type"), required(r, "title"), r.FormValue("statement"), r.FormValue("explanation"), r.FormValue("proof_outline"), r.FormValue("proof_full"), formInt(r, "source_page", 0))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	redirect(w, r, "/admin?message=theory")
}

func (a *App) createCard(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := a.db.Exec(r.Context(), `
		insert into cards (topic_id, theory_item_id, type, front, back, hint, difficulty)
		values ($1, nullif($2, 0), $3, $4, $5, $6, $7)
	`, formInt(r, "topic_id", 0), formInt(r, "theory_item_id", 0), required(r, "type"), required(r, "front"), required(r, "back"), r.FormValue("hint"), formInt(r, "difficulty", 1))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	redirect(w, r, "/admin?message=card")
}

func (a *App) createQuestion(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tags := splitTags(r.FormValue("tags"))
	_, err := a.db.Exec(r.Context(), `
		insert into questions (topic_id, theory_item_id, type, exam_kind, text, expected_answer, short_answer, difficulty, tags)
		values ($1, nullif($2, 0), $3, $4, $5, $6, $7, $8, $9)
	`, formInt(r, "topic_id", 0), formInt(r, "theory_item_id", 0), required(r, "type"), required(r, "exam_kind"), required(r, "text"), r.FormValue("expected_answer"), r.FormValue("short_answer"), formInt(r, "difficulty", 1), tags)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	redirect(w, r, "/admin?message=question")
}

func (a *App) createProofGame(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := a.db.Exec(r.Context(), `
		insert into proof_choice_games (theory_item_id, question, correct_outline, wrong_outline_1, wrong_outline_2, wrong_outline_3, explanation)
		values ($1, $2, $3, $4, $5, $6, $7)
	`, formInt(r, "theory_item_id", 0), required(r, "question"), required(r, "correct_outline"), required(r, "wrong_outline_1"), required(r, "wrong_outline_2"), required(r, "wrong_outline_3"), r.FormValue("explanation"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	redirect(w, r, "/admin?message=proof_game")
}

func (a *App) stats(ctx context.Context) (map[string]int, error) {
	stats := map[string]int{}
	for _, table := range []string{"topics", "theory_items", "cards", "questions", "proof_choice_games", "exam_blueprints", "attempts"} {
		var count int
		query := "select count(*) from " + table
		if table == "questions" || table == "proof_choice_games" {
			query += " where source = 'curated'"
		}
		if err := a.db.QueryRow(ctx, query).Scan(&count); err != nil {
			return nil, err
		}
		stats[table] = count
	}
	return stats, nil
}

func (a *App) listTopics(ctx context.Context) ([]Topic, error) {
	rows, err := a.db.Query(ctx, `
		select id, title, chapter, description, difficulty, semester, sort_order
		from topics
		order by semester, sort_order, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []Topic
	for rows.Next() {
		var topic Topic
		if err := rows.Scan(&topic.ID, &topic.Title, &topic.Chapter, &topic.Description, &topic.Difficulty, &topic.Semester, &topic.SortOrder); err != nil {
			return nil, err
		}
		topics = append(topics, topic)
	}
	return topics, rows.Err()
}

func (a *App) listTheoryItems(ctx context.Context) ([]TheoryItem, error) {
	rows, err := a.db.Query(ctx, `
		select id, topic_id, type, title, statement, explanation, proof_outline, proof_full, coalesce(source_page, 0)
		from theory_items
		where title like 'Определение:%' or title like 'Теорема:%' or title like 'Критерий:%' or title like 'Пример:%' or title like 'Полное доказательство:%'
		order by topic_id,
			case type when 'definition' then 1 when 'theorem' then 2 when 'criterion' then 3 when 'property' then 4 when 'example' then 5 when 'proof' then 6 else 7 end,
			id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []TheoryItem
	for rows.Next() {
		var item TheoryItem
		if err := rows.Scan(&item.ID, &item.TopicID, &item.Type, &item.Title, &item.Statement, &item.Explanation, &item.ProofOutline, &item.ProofFull, &item.SourcePage); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) getTopic(ctx context.Context, id int) (Topic, error) {
	var topic Topic
	err := a.db.QueryRow(ctx, `
		select id, title, chapter, description, difficulty
		from topics
		where id = $1
	`, id).Scan(&topic.ID, &topic.Title, &topic.Chapter, &topic.Description, &topic.Difficulty)
	if err != nil {
		return Topic{}, err
	}

	rows, err := a.db.Query(ctx, `
		select id, topic_id, type, title, statement, explanation, proof_outline, proof_full, coalesce(source_page, 0)
		from theory_items
		where topic_id = $1 and (title like 'Определение:%' or title like 'Теорема:%' or title like 'Критерий:%' or title like 'Пример:%' or title like 'Полное доказательство:%')
		order by case type when 'definition' then 1 when 'theorem' then 2 when 'criterion' then 3 when 'property' then 4 when 'example' then 5 when 'proof' then 6 else 7 end, id
	`, id)
	if err != nil {
		return Topic{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var item TheoryItem
		if err := rows.Scan(&item.ID, &item.TopicID, &item.Type, &item.Title, &item.Statement, &item.Explanation, &item.ProofOutline, &item.ProofFull, &item.SourcePage); err != nil {
			return Topic{}, err
		}
		topic.Items = append(topic.Items, item)
	}
	return topic, rows.Err()
}

func (a *App) listCards(ctx context.Context, topicID string) ([]Card, error) {
	query := `
		select c.id, c.topic_id, c.type, c.front, c.back, c.hint, c.difficulty, t.title
		from cards c
		join topics t on t.id = c.topic_id
		where $1 = '' or c.topic_id::text = $1
		order by c.difficulty, c.id
	`
	rows, err := a.db.Query(ctx, query, topicID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []Card
	for rows.Next() {
		var card Card
		if err := rows.Scan(&card.ID, &card.TopicID, &card.Type, &card.Front, &card.Back, &card.Hint, &card.Difficulty, &card.TopicTitle); err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}
	return cards, rows.Err()
}

func (a *App) randomCard(ctx context.Context) (Card, error) {
	var card Card
	err := a.db.QueryRow(ctx, `
		select c.id, c.topic_id, c.type, c.front, c.back, c.hint, c.difficulty, t.title
		from cards c
		join topics t on t.id = c.topic_id
		order by random()
		limit 1
	`).Scan(&card.ID, &card.TopicID, &card.Type, &card.Front, &card.Back, &card.Hint, &card.Difficulty, &card.TopicTitle)
	return card, err
}

func (a *App) randomProofGame(ctx context.Context) (ProofChoice, error) {
	var game ProofChoice
	err := a.db.QueryRow(ctx, `
		select pg.id, pg.theory_item_id, pg.question, pg.correct_outline, pg.wrong_outline_1,
		       pg.wrong_outline_2, pg.wrong_outline_3, pg.explanation, t.title, ti.title
		from proof_choice_games pg
		join theory_items ti on ti.id = pg.theory_item_id
		join topics t on t.id = ti.topic_id
		where pg.source = 'curated'
		order by random()
		limit 1
	`).Scan(&game.ID, &game.TheoryItemID, &game.Question, &game.CorrectOutline, &game.Wrong1, &game.Wrong2, &game.Wrong3, &game.Explanation, &game.TopicTitle, &game.TheoryTitle)
	if err != nil {
		return ProofChoice{}, err
	}

	game.Options = []ProofOption{
		{Text: game.CorrectOutline, Correct: true},
		{Text: game.Wrong1},
		{Text: game.Wrong2},
		{Text: game.Wrong3},
	}
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(game.Options), func(i, j int) {
		game.Options[i], game.Options[j] = game.Options[j], game.Options[i]
	})
	return game, nil
}

func (a *App) randomQuestion(ctx context.Context, mode string) (Question, error) {
	var question Question
	err := a.db.QueryRow(ctx, `
		select q.id, q.topic_id, coalesce(q.theory_item_id, 0), q.type, q.exam_kind, q.text, q.expected_answer,
		       q.short_answer, q.difficulty, q.tags, t.title
		from questions q
		join topics t on t.id = q.topic_id
		where q.source = 'curated' and ($1 = '' or $1 = any(q.tags) or q.type = $1)
		order by random()
		limit 1
	`, mode).Scan(&question.ID, &question.TopicID, &question.TheoryItemID, &question.Type, &question.ExamKind, &question.Text, &question.ExpectedAnswer, &question.ShortAnswer, &question.Difficulty, &question.Tags, &question.TopicTitle)
	if err != nil && mode != "" {
		return a.randomQuestion(ctx, "")
	}
	return question, err
}

func (a *App) variantQuestions(ctx context.Context, examKind string, limit int, semester int) ([]Question, error) {
	rows, err := a.db.Query(ctx, `
		select id, topic_id, theory_item_id, type, exam_kind, text, expected_answer,
		       short_answer, difficulty, tags, title
		from (
			select q.id, q.topic_id, coalesce(q.theory_item_id, 0) as theory_item_id, q.type, q.exam_kind,
			       q.text, q.expected_answer, q.short_answer, q.difficulty, q.tags, t.title,
			       row_number() over (partition by q.topic_id order by random()) as topic_number
			from questions q
			join topics t on t.id = q.topic_id
			where q.source = 'curated' and ($1 = '' or q.exam_kind = $1) and ($3 = 0 or t.semester = $3)
		) selected
		where topic_number = 1
		order by random()
		limit $2
	`, examKind, limit, semester)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var questions []Question
	for rows.Next() {
		var question Question
		if err := rows.Scan(&question.ID, &question.TopicID, &question.TheoryItemID, &question.Type, &question.ExamKind, &question.Text, &question.ExpectedAnswer, &question.ShortAnswer, &question.Difficulty, &question.Tags, &question.TopicTitle); err != nil {
			return nil, err
		}
		questions = append(questions, question)
	}
	return questions, rows.Err()
}

func (a *App) progressRows(ctx context.Context, studentID int64) ([]ProgressRow, error) {
	rows, err := a.db.Query(ctx, `
		select mode, grade, count(*) as attempts, max(created_at)
		from attempts
		where demo_student_id = $1
		group by mode, grade
		order by max(created_at) desc
	`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var progress []ProgressRow
	for rows.Next() {
		var row ProgressRow
		var lastAttempt time.Time
		if err := rows.Scan(&row.Mode, &row.Grade, &row.Attempts, &lastAttempt); err != nil {
			return nil, err
		}
		row.LastAttemptAt = lastAttempt.Format("02.01.2006 15:04")
		progress = append(progress, row)
	}
	return progress, rows.Err()
}

func migrate(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, schemaSQL)
	return err
}

func seed(ctx context.Context, db *pgxpool.Pool) error {
	var count int
	if err := db.QueryRow(ctx, "select count(*) from topics").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		_, err := db.Exec(ctx, enrichmentSQL)
		if err != nil {
			return err
		}
		_, err = db.Exec(ctx, catalogSQL)
		if err != nil {
			return err
		}
		_, err = db.Exec(ctx, fullTheorySQL)
		if err != nil {
			return fmt.Errorf("full theory: %w", err)
		}
		_, err = db.Exec(ctx, expansionSQL)
		if err != nil {
			return err
		}
		_, err = db.Exec(ctx, taskPackSQL)
		if err != nil {
			return err
		}
		_, err = db.Exec(ctx, notesSQL)
		if err != nil {
			return err
		}
		if _, err = db.Exec(ctx, safeTaskSQL); err != nil {
			return fmt.Errorf("audited tasks: %w", err)
		}
		if _, err = db.Exec(ctx, repairSQL); err != nil {
			return fmt.Errorf("repair tasks: %w", err)
		}
		_, err = db.Exec(ctx, qualitySQL)
		if err != nil {
			return fmt.Errorf("quality: %w", err)
		}
		if err = seedCuratedContent(ctx, db); err != nil {
			return fmt.Errorf("curated content: %w", err)
		}
		return nil
	}
	_, err := db.Exec(ctx, seedSQL)
	if err != nil {
		return fmt.Errorf("seed demo data: %w", err)
	}
	_, err = db.Exec(ctx, enrichmentSQL)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, catalogSQL)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, fullTheorySQL)
	if err != nil {
		return fmt.Errorf("full theory: %w", err)
	}
	_, err = db.Exec(ctx, expansionSQL)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, taskPackSQL)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, notesSQL)
	if err != nil {
		return err
	}
	if _, err = db.Exec(ctx, safeTaskSQL); err != nil {
		return fmt.Errorf("audited tasks: %w", err)
	}
	if _, err = db.Exec(ctx, repairSQL); err != nil {
		return fmt.Errorf("repair tasks: %w", err)
	}
	_, err = db.Exec(ctx, qualitySQL)
	if err != nil {
		return fmt.Errorf("quality: %w", err)
	}
	if err = seedCuratedContent(ctx, db); err != nil {
		return fmt.Errorf("curated content: %w", err)
	}
	return nil
}

func execSQLBatch(ctx context.Context, db *pgxpool.Pool, sql, name string) error {
	for index, statement := range strings.Split(sql, ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := db.Exec(ctx, statement); err != nil {
			position := ""
			var pgErr *pgconn.PgError
			preview := statement
			if errors.As(err, &pgErr) {
				pos := int(pgErr.Position)
				position = fmt.Sprintf(" position=%d", pos)
				start, end := pos-180, pos+180
				if start < 0 {
					start = 0
				}
				if end > len(statement) {
					end = len(statement)
				}
				preview = statement[start:pos-1] + "<ERROR>" + statement[pos-1:end]
			} else if len(preview) > 900 {
				preview = preview[:900]
			}
			return fmt.Errorf("%s statement %d:%s %w; sql=%s", name, index+1, position, err, preview)
		}
	}
	return nil
}

func required(r *http.Request, name string) string {
	return strings.TrimSpace(r.FormValue(name))
}

func formInt(r *http.Request, name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(r.FormValue(name)))
	if err != nil {
		return fallback
	}
	return value
}

func splitTags(value string) []string {
	parts := strings.Split(value, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func semesterParam(r *http.Request) int {
	value, _ := strconv.Atoi(r.URL.Query().Get("semester"))
	if value != 1 && value != 2 {
		return 0
	}
	return value
}

func semesterTitle(value int) string {
	if value == 1 {
		return "1 семестр"
	}
	if value == 2 {
		return "2 семестр"
	}
	return "все семестры"
}

func redirect(w http.ResponseWriter, r *http.Request, fallback string) {
	if fallback == "" {
		fallback = "/"
	}
	http.Redirect(w, r, fallback, http.StatusSeeOther)
}
