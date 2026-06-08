package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed templates/*.html static/*
var assets embed.FS

const demoStudentID = 1

type App struct {
	db *pgxpool.Pool
}

type Topic struct {
	ID          int
	Title       string
	Chapter     string
	Description string
	Difficulty  int
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
}

type ProofChoiceView struct {
	ProofChoice
	Options []ProofOption
}

type ProofOption struct {
	Text    string
	Correct bool
}

type Question struct {
	ID             int
	TopicID        int
	TheoryItemID   int
	Type           string
	Text           string
	ExpectedAnswer string
	ShortAnswer    string
	Difficulty     int
	Tags           []string
	TopicTitle     string
}

type ProgressRow struct {
	Mode          string
	Grade         string
	Attempts      int
	LastAttemptAt string
}

type PageData struct {
	Title        string
	Active       string
	Topics       []Topic
	Topic        Topic
	Cards        []Card
	Card         Card
	ProofGame    ProofChoiceView
	Questions    []Question
	Question     Question
	Progress     []ProgressRow
	Stats        map[string]int
	ErrorMessage string
}

func main() {
	ctx := context.Background()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:postgres@localhost:5432/matan_trainer?sslmode=disable"
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
	if err := seed(ctx, db); err != nil {
		log.Fatalf("seed: %v", err)
	}

	app := &App{db: db}

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.FileServerFS(assets))
	mux.HandleFunc("GET /", app.home)
	mux.HandleFunc("GET /topics", app.topics)
	mux.HandleFunc("GET /topic/", app.topic)
	mux.HandleFunc("GET /cards", app.cards)
	mux.HandleFunc("GET /formulations", app.formulations)
	mux.HandleFunc("GET /proof-game", app.proofGame)
	mux.HandleFunc("GET /oral", app.oral)
	mux.HandleFunc("GET /variant", app.variant)
	mux.HandleFunc("GET /progress", app.progress)
	mux.HandleFunc("POST /attempts", app.recordAttempt)

	addr := ":8080"
	log.Printf("matan trainer listening on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func (a *App) render(w http.ResponseWriter, name string, data PageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl, err := template.ParseFS(assets, "templates/layout.html", "templates/"+name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats := map[string]int{}
	for _, table := range []string{"topics", "cards", "questions", "proof_choice_games"} {
		var count int
		_ = a.db.QueryRow(ctx, "select count(*) from "+table).Scan(&count)
		stats[table] = count
	}
	a.render(w, "home.html", PageData{Title: "Тренажер матана", Active: "home", Stats: stats})
}

func (a *App) topics(w http.ResponseWriter, r *http.Request) {
	topics, err := a.listTopics(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, "topics.html", PageData{Title: "Темы", Active: "topics", Topics: topics})
}

func (a *App) topic(w http.ResponseWriter, r *http.Request) {
	idText := strings.TrimPrefix(r.URL.Path, "/topic/")
	id, err := strconv.Atoi(idText)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	topic, err := a.getTopic(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a.render(w, "topic.html", PageData{Title: topic.Title, Active: "topics", Topic: topic})
}

func (a *App) cards(w http.ResponseWriter, r *http.Request) {
	cards, err := a.listCards(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, "cards.html", PageData{Title: "Карточки", Active: "cards", Cards: cards})
}

func (a *App) formulations(w http.ResponseWriter, r *http.Request) {
	card, err := a.randomCard(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, "formulations.html", PageData{Title: "Формулировки", Active: "formulations", Card: card})
}

func (a *App) proofGame(w http.ResponseWriter, r *http.Request) {
	game, err := a.randomProofGame(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, "proof_game.html", PageData{Title: "Выбери доказательство", Active: "proof-game", ProofGame: game})
}

func (a *App) oral(w http.ResponseWriter, r *http.Request) {
	q, err := a.randomQuestion(r.Context(), "oral")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, "oral.html", PageData{Title: "Устный тест", Active: "oral", Question: q})
}

func (a *App) variant(w http.ResponseWriter, r *http.Request) {
	questions, err := a.variantQuestions(r.Context(), 6)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, "variant.html", PageData{Title: "Письменный вариант", Active: "variant", Questions: questions})
}

func (a *App) progress(w http.ResponseWriter, r *http.Request) {
	rows, err := a.progressRows(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.render(w, "progress.html", PageData{Title: "Прогресс", Active: "progress", Progress: rows})
}

func (a *App) recordAttempt(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	targetType := r.FormValue("target_type")
	targetID, _ := strconv.Atoi(r.FormValue("target_id"))
	mode := r.FormValue("mode")
	grade := r.FormValue("grade")
	notes := r.FormValue("notes")
	if targetType == "" || targetID == 0 || mode == "" || grade == "" {
		http.Error(w, "missing attempt fields", http.StatusBadRequest)
		return
	}

	_, err := a.db.Exec(r.Context(), `
		insert into attempts (demo_student_id, target_type, target_id, mode, grade, notes)
		values ($1, $2, $3, $4, $5, $6)
	`, demoStudentID, targetType, targetID, mode, grade, notes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	redirect := r.FormValue("redirect")
	if redirect == "" {
		redirect = "/progress"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (a *App) listTopics(ctx context.Context) ([]Topic, error) {
	rows, err := a.db.Query(ctx, `
		select id, title, chapter, description, difficulty
		from topics
		order by chapter, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []Topic
	for rows.Next() {
		var t Topic
		if err := rows.Scan(&t.ID, &t.Title, &t.Chapter, &t.Description, &t.Difficulty); err != nil {
			return nil, err
		}
		topics = append(topics, t)
	}
	return topics, rows.Err()
}

func (a *App) getTopic(ctx context.Context, id int) (Topic, error) {
	var t Topic
	err := a.db.QueryRow(ctx, `
		select id, title, chapter, description, difficulty
		from topics
		where id = $1
	`, id).Scan(&t.ID, &t.Title, &t.Chapter, &t.Description, &t.Difficulty)
	if err != nil {
		return Topic{}, err
	}

	rows, err := a.db.Query(ctx, `
		select id, topic_id, type, title, statement, explanation, proof_outline, proof_full, coalesce(source_page, 0)
		from theory_items
		where topic_id = $1
		order by id
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
		t.Items = append(t.Items, item)
	}
	return t, rows.Err()
}

func (a *App) listCards(ctx context.Context) ([]Card, error) {
	rows, err := a.db.Query(ctx, `
		select c.id, c.topic_id, c.type, c.front, c.back, c.hint, c.difficulty, t.title
		from cards c
		join topics t on t.id = c.topic_id
		order by c.difficulty, c.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []Card
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.ID, &c.TopicID, &c.Type, &c.Front, &c.Back, &c.Hint, &c.Difficulty, &c.TopicTitle); err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

func (a *App) randomCard(ctx context.Context) (Card, error) {
	var c Card
	err := a.db.QueryRow(ctx, `
		select c.id, c.topic_id, c.type, c.front, c.back, c.hint, c.difficulty, t.title
		from cards c
		join topics t on t.id = c.topic_id
		order by random()
		limit 1
	`).Scan(&c.ID, &c.TopicID, &c.Type, &c.Front, &c.Back, &c.Hint, &c.Difficulty, &c.TopicTitle)
	return c, err
}

func (a *App) randomProofGame(ctx context.Context) (ProofChoiceView, error) {
	var p ProofChoiceView
	err := a.db.QueryRow(ctx, `
		select pg.id, pg.theory_item_id, pg.question, pg.correct_outline, pg.wrong_outline_1,
		       pg.wrong_outline_2, pg.wrong_outline_3, pg.explanation, t.title, ti.title
		from proof_choice_games pg
		join theory_items ti on ti.id = pg.theory_item_id
		join topics t on t.id = ti.topic_id
		order by random()
		limit 1
	`).Scan(&p.ID, &p.TheoryItemID, &p.Question, &p.CorrectOutline, &p.Wrong1, &p.Wrong2, &p.Wrong3, &p.Explanation, &p.TopicTitle, &p.TheoryTitle)
	if err != nil {
		return ProofChoiceView{}, err
	}

	p.Options = []ProofOption{
		{Text: p.CorrectOutline, Correct: true},
		{Text: p.Wrong1},
		{Text: p.Wrong2},
		{Text: p.Wrong3},
	}
	rand.New(rand.NewSource(time.Now().UnixNano())).Shuffle(len(p.Options), func(i, j int) {
		p.Options[i], p.Options[j] = p.Options[j], p.Options[i]
	})
	return p, nil
}

func (a *App) randomQuestion(ctx context.Context, mode string) (Question, error) {
	query := `
		select q.id, q.topic_id, q.theory_item_id, q.type, q.text, q.expected_answer, q.short_answer,
		       q.difficulty, q.tags, t.title
		from questions q
		join topics t on t.id = q.topic_id
		where $1 = '' or q.type = $1 or $1 = any(q.tags)
		order by random()
		limit 1
	`
	var q Question
	err := a.db.QueryRow(ctx, query, mode).Scan(&q.ID, &q.TopicID, &q.TheoryItemID, &q.Type, &q.Text, &q.ExpectedAnswer, &q.ShortAnswer, &q.Difficulty, &q.Tags, &q.TopicTitle)
	if err != nil && mode != "" {
		return a.randomQuestion(ctx, "")
	}
	return q, err
}

func (a *App) variantQuestions(ctx context.Context, limit int) ([]Question, error) {
	rows, err := a.db.Query(ctx, `
		select q.id, q.topic_id, q.theory_item_id, q.type, q.text, q.expected_answer, q.short_answer,
		       q.difficulty, q.tags, t.title
		from questions q
		join topics t on t.id = q.topic_id
		order by random()
		limit $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var questions []Question
	for rows.Next() {
		var q Question
		if err := rows.Scan(&q.ID, &q.TopicID, &q.TheoryItemID, &q.Type, &q.Text, &q.ExpectedAnswer, &q.ShortAnswer, &q.Difficulty, &q.Tags, &q.TopicTitle); err != nil {
			return nil, err
		}
		questions = append(questions, q)
	}
	return questions, rows.Err()
}

func (a *App) progressRows(ctx context.Context) ([]ProgressRow, error) {
	rows, err := a.db.Query(ctx, `
		select mode, grade, count(*) as attempts, max(created_at)
		from attempts
		where demo_student_id = $1
		group by mode, grade
		order by max(created_at) desc
	`, demoStudentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var progress []ProgressRow
	for rows.Next() {
		var row ProgressRow
		var last time.Time
		if err := rows.Scan(&row.Mode, &row.Grade, &row.Attempts, &last); err != nil {
			return nil, err
		}
		row.LastAttemptAt = last.Format("02.01.2006 15:04")
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
		return nil
	}
	_, err := db.Exec(ctx, seedSQL)
	if err != nil {
		return fmt.Errorf("seed demo data: %w", err)
	}
	return nil
}
