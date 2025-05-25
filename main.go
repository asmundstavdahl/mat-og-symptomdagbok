package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type templateData struct {
	MealOptions    []string
	SymptomOptions []string
	Now            string
	Meals          []Meal
	Symptoms       []Symptom
}

// crossCorrPageHandler displays the cross-correlation UI.
func crossCorrPageHandler(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	data := struct{ Start, End string }{
		Start: now.AddDate(0, 0, -14).Format("2006-01-02"),
		End:   now.Format("2006-01-02"),
	}
	if err := templates.ExecuteTemplate(w, "crosscorr.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// crossCorrDataHandler returns JSON data of cross-correlation between meals and symptoms.
func crossCorrDataHandler(w http.ResponseWriter, r *http.Request) {
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" || end == "" {
		http.Error(w, "start og end må spesifiseres", http.StatusBadRequest)
		return
	}
	layout := "2006-01-02"
	startDate, err := time.Parse(layout, start)
	if err != nil {
		http.Error(w, "ugyldig startdato", http.StatusBadRequest)
		return
	}
	endDate, err := time.Parse(layout, end)
	if err != nil {
		http.Error(w, "ugyldig sluttdato", http.StatusBadRequest)
		return
	}

	// Get daily meal counts
	mealRows, err := db.Query(
		"SELECT DATE(timestamp) as day, COUNT(*) "+
			"FROM meals WHERE DATE(timestamp) BETWEEN ? AND ? "+
			"GROUP BY day ORDER BY day", start, end)
	if err != nil {
		http.Error(w, "kunne ikke hente rapportdata", http.StatusInternalServerError)
		return
	}
	defer mealRows.Close()
	mealCounts := make(map[string]int)
	for mealRows.Next() {
		var day string
		var count int
		if err := mealRows.Scan(&day, &count); err != nil {
			http.Error(w, "feil ved scanning", http.StatusInternalServerError)
			return
		}
		mealCounts[day] = count
	}

	// Get daily symptom counts
	sympRows, err := db.Query(
		"SELECT DATE(timestamp) as day, COUNT(*) "+
			"FROM symptoms WHERE DATE(timestamp) BETWEEN ? AND ? "+
			"GROUP BY day ORDER BY day", start, end)
	if err != nil {
		http.Error(w, "kunne ikke hente rapportdata", http.StatusInternalServerError)
		return
	}
	defer sympRows.Close()
	sympCounts := make(map[string]int)
	for sympRows.Next() {
		var day string
		var count int
		if err := sympRows.Scan(&day, &count); err != nil {
			http.Error(w, "feil ved scanning", http.StatusInternalServerError)
			return
		}
		sympCounts[day] = count
	}

	// Build aligned slices
	var days []string
	var meals []float64
	var symptoms []float64
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		dayStr := d.Format(layout)
		days = append(days, dayStr)
		meals = append(meals, float64(mealCounts[dayStr]))
		symptoms = append(symptoms, float64(sympCounts[dayStr]))
	}

	// Compute cross-correlation for lags -7 to +7
	maxLag := 7
	type CorrResult struct {
		Lag   int     `json:"lag"`
		Value float64 `json:"value"`
	}
	var results []CorrResult
	n := len(meals)
	mean := func(x []float64) float64 {
		s := 0.0
		for _, v := range x {
			s += v
		}
		return s / float64(len(x))
	}
	std := func(x []float64, m float64) float64 {
		s := 0.0
		for _, v := range x {
			s += (v - m) * (v - m)
		}
		return (s / float64(len(x)))
	}
	meanMeals := mean(meals)
	meanSymptoms := mean(symptoms)
	stdMeals := std(meals, meanMeals)
	stdSymptoms := std(symptoms, meanSymptoms)
	for lag := -maxLag; lag <= maxLag; lag++ {
		var xs, ys []float64
		for i := 0; i < n; i++ {
			j := i + lag
			if j < 0 || j >= n {
				continue
			}
			xs = append(xs, meals[i])
			ys = append(ys, symptoms[j])
		}
		if len(xs) == 0 {
			results = append(results, CorrResult{Lag: lag, Value: 0})
			continue
		}
		mx := mean(xs)
		my := mean(ys)
		var num, denomX, denomY float64
		for i := range xs {
			num += (xs[i] - mx) * (ys[i] - my)
			denomX += (xs[i] - mx) * (xs[i] - mx)
			denomY += (ys[i] - my) * (ys[i] - my)
		}
		denom := (denomX * denomY)
		corr := 0.0
		if denom > 0 {
			corr = num / (float64(len(xs)) * (denomX/float64(len(xs))) * (denomY/float64(len(xs))))
			corr = num / (math.Sqrt(denomX) * math.Sqrt(denomY))
		}
		results = append(results, CorrResult{Lag: lag, Value: corr})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Lags  []int     `json:"lags"`
		Values []float64 `json:"values"`
	}{
		Lags: func() []int {
			lags := make([]int, 0, len(results))
			for _, r := range results {
				lags = append(lags, r.Lag)
			}
			return lags
		}(),
		Values: func() []float64 {
			vals := make([]float64, 0, len(results))
			for _, r := range results {
				vals = append(vals, r.Value)
			}
			return vals
		}(),
	})
}

var (
	templates *template.Template
	db        *sql.DB
)

// Meal represents a recorded meal entry.
type Meal struct {
	ID          int       `json:"id"`
	Items       string    `json:"items"`
	Timestamp   time.Time `json:"timestamp"`
	Note        string    `json:"note"`
	DisplayTime string    `json:"-"`
	InputTime   string    `json:"-"`
}

// Symptom represents a recorded symptom entry.
type Symptom struct {
	ID          int       `json:"id"`
	Description string    `json:"description"`
	Timestamp   time.Time `json:"timestamp"`
	Note        string    `json:"note"`
	DisplayTime string    `json:"-"`
	InputTime   string    `json:"-"`
}

func main() {
	port := flag.Int("port", 8080, "Port to run the server on")
	flag.Parse()

	var err error
	db, err = sql.Open("sqlite3", "data.db")
	if err != nil {
		log.Fatalf("database connection error: %v", err)
	}
	defer db.Close()

	if err := migrate(db); err != nil {
		log.Fatalf("migration error: %v", err)
	}

	templates, err = template.ParseGlob(filepath.Join("templates", "*.html"))
	if err != nil {
		log.Fatalf("parsing templates error: %v", err)
	}

	// Serve static files (for plotly.min.js)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/meals", mealsHandler)
	http.HandleFunc("/symptoms", symptomsHandler)

	http.HandleFunc("/meals/edit", editMealHandler)
	http.HandleFunc("/meals/update", updateMealHandler)
	http.HandleFunc("/meals/delete", deleteMealHandler)
	http.HandleFunc("/symptoms/edit", editSymptomHandler)
	http.HandleFunc("/symptoms/update", updateSymptomHandler)
	http.HandleFunc("/symptoms/delete", deleteSymptomHandler)
	http.HandleFunc("/export", exportHandler)
	http.HandleFunc("/report", reportPageHandler)
	http.HandleFunc("/report/data", reportDataHandler)
	http.HandleFunc("/crosscorr", crossCorrPageHandler)
	http.HandleFunc("/crosscorr/data", crossCorrDataHandler)

	log.Printf("Server starting on :%d\n", *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// reportPageHandler displays the reporting UI for filtering and visualization.
func reportPageHandler(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	data := struct{ Start, End string }{
		Start: now.AddDate(0, 0, -7).Format("2006-01-02"),
		End:   now.Format("2006-01-02"),
	}
	if err := templates.ExecuteTemplate(w, "report.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// reportDataHandler returns JSON data of meal and symptom counts per day in a time range.
func reportDataHandler(w http.ResponseWriter, r *http.Request) {
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" || end == "" {
		http.Error(w, "start og end må spesifiseres", http.StatusBadRequest)
		return
	}
	mealRows, err := db.Query(
		"SELECT DATE(timestamp) as day, COUNT(*) "+
			"FROM meals WHERE DATE(timestamp) BETWEEN ? AND ? "+
			"GROUP BY day ORDER BY day", start, end)
	if err != nil {
		http.Error(w, "kunne ikke hente rapportdata", http.StatusInternalServerError)
		return
	}
	defer mealRows.Close()
	mealCounts := make(map[string]int)
	for mealRows.Next() {
		var day string
		var count int
		if err := mealRows.Scan(&day, &count); err != nil {
			http.Error(w, "feil ved scanning", http.StatusInternalServerError)
			return
		}
		mealCounts[day] = count
	}
	sympRows, err := db.Query(
		"SELECT DATE(timestamp) as day, COUNT(*) "+
			"FROM symptoms WHERE DATE(timestamp) BETWEEN ? AND ? "+
			"GROUP BY day ORDER BY day", start, end)
	if err != nil {
		http.Error(w, "kunne ikke hente rapportdata", http.StatusInternalServerError)
		return
	}
	defer sympRows.Close()
	sympCounts := make(map[string]int)
	for sympRows.Next() {
		var day string
		var count int
		if err := sympRows.Scan(&day, &count); err != nil {
			http.Error(w, "feil ved scanning", http.StatusInternalServerError)
			return
		}
		sympCounts[day] = count
	}
	layout := "2006-01-02"
	startDate, err := time.Parse(layout, start)
	if err != nil {
		http.Error(w, "ugyldig startdato", http.StatusBadRequest)
		return
	}
	endDate, err := time.Parse(layout, end)
	if err != nil {
		http.Error(w, "ugyldig sluttdato", http.StatusBadRequest)
		return
	}
	var days []string
	var meals []int
	var symptoms []int
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		dayStr := d.Format(layout)
		days = append(days, dayStr)
		meals = append(meals, mealCounts[dayStr])
		symptoms = append(symptoms, sympCounts[dayStr])
	}
	result := struct {
		Days     []string `json:"days"`
		Meals    []int    `json:"meals"`
		Symptoms []int    `json:"symptoms"`
	}{days, meals, symptoms}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		http.Error(w, "feil ved encoding av JSON", http.StatusInternalServerError)
		return
	}
}

func migrate(db *sql.DB) error {
	entries, err := os.ReadDir("migrations")
	if err != nil {
		return err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)
	for _, fname := range files {
		path := filepath.Join("migrations", fname)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(content)); err != nil {
			return err
		}
	}
	return nil
}

// getAllMeals retrieves all meals from the database.
func getAllMeals() ([]Meal, error) {
	rows, err := db.Query("SELECT id, items, timestamp, note FROM meals ORDER BY timestamp DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var meals []Meal
	for rows.Next() {
		var m Meal
		var ts string
		if err := rows.Scan(&m.ID, &m.Items, &ts, &m.Note); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return nil, err
		}
		m.Timestamp = t
		m.DisplayTime = t.Format("2006-01-02 15:04")
		m.InputTime = t.Local().Format("2006-01-02T15:04")
		meals = append(meals, m)
	}
	return meals, nil
}

// getAllSymptoms retrieves all symptoms from the database.
func getAllSymptoms() ([]Symptom, error) {
	rows, err := db.Query("SELECT id, description, timestamp, note FROM symptoms ORDER BY timestamp DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var symptoms []Symptom
	for rows.Next() {
		var s Symptom
		var ts string
		if err := rows.Scan(&s.ID, &s.Description, &ts, &s.Note); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return nil, err
		}
		s.Timestamp = t
		s.DisplayTime = t.Format("2006-01-02 15:04")
		s.InputTime = t.Local().Format("2006-01-02T15:04")
		symptoms = append(symptoms, s)
	}
	return symptoms, nil
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	meals, err := getAllMeals()
	if err != nil {
		http.Error(w, "kunne ikke hente måltider", http.StatusInternalServerError)
		return
	}
	symptoms, err := getAllSymptoms()
	if err != nil {
		http.Error(w, "kunne ikke hente symptomer", http.StatusInternalServerError)
		return
	}
	data := templateData{
		MealOptions:    []string{"Brød", "Melk", "Ost"},
		SymptomOptions: []string{"Hodepine", "Kvalme", "Tretthet"},
		Now:            time.Now().Format("2006-01-02T15:04"),
		Meals:          meals,
		Symptoms:       symptoms,
	}
	if err := templates.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func mealsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	items := r.FormValue("items")
	timestampStr := r.FormValue("timestamp")
	note := r.FormValue("note")

	t, err := time.Parse("2006-01-02T15:04", timestampStr)
	if err != nil {
		http.Error(w, "ugyldig tidspunkt", http.StatusBadRequest)
		return
	}
	_, err = db.Exec("INSERT INTO meals (items, timestamp, note) VALUES (?, ?, ?)", items, t.Format(time.RFC3339), note)
	if err != nil {
		http.Error(w, "feil ved lagring", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func symptomsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	description := r.FormValue("description")
	timestampStr := r.FormValue("timestamp")
	note := r.FormValue("note")

	t, err := time.Parse("2006-01-02T15:04", timestampStr)
	if err != nil {
		http.Error(w, "ugyldig tidspunkt", http.StatusBadRequest)
		return
	}
	_, err = db.Exec("INSERT INTO symptoms (description, timestamp, note) VALUES (?, ?, ?)", description, t.Format(time.RFC3339), note)
	if err != nil {
		http.Error(w, "feil ved lagring", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// editMealHandler displays a form to edit an existing meal.
func editMealHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	row := db.QueryRow("SELECT id, items, timestamp, note FROM meals WHERE id = ?", id)
	var m Meal
	var ts string
	if err := row.Scan(&m.ID, &m.Items, &ts, &m.Note); err != nil {
		http.Error(w, "måltid ikke funnet", http.StatusNotFound)
		return
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		http.Error(w, "ugyldig tidspunkt", http.StatusInternalServerError)
		return
	}
	m.Timestamp = t
	m.DisplayTime = t.Format("2006-01-02 15:04")
	m.InputTime = t.Local().Format("2006-01-02T15:04")
	data := struct {
		MealOptions []string
		Meal        Meal
	}{
		MealOptions: []string{"Brød", "Melk", "Ost"},
		Meal:        m,
	}
	if err := templates.ExecuteTemplate(w, "edit_meal.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// updateMealHandler processes the meal update form.
func updateMealHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	id := r.FormValue("id")
	items := r.FormValue("items")
	timestampStr := r.FormValue("timestamp")
	note := r.FormValue("note")
	t, err := time.Parse("2006-01-02T15:04", timestampStr)
	if err != nil {
		http.Error(w, "ugyldig tidspunkt", http.StatusBadRequest)
		return
	}
	_, err = db.Exec("UPDATE meals SET items = ?, timestamp = ?, note = ? WHERE id = ?", items, t.Format(time.RFC3339), note, id)
	if err != nil {
		http.Error(w, "feil ved oppdatering", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// deleteMealHandler deletes a meal entry.
func deleteMealHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	id := r.FormValue("id")
	_, err := db.Exec("DELETE FROM meals WHERE id = ?", id)
	if err != nil {
		http.Error(w, "feil ved sletting", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// editSymptomHandler displays a form to edit an existing symptom.
func editSymptomHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	row := db.QueryRow("SELECT id, description, timestamp, note FROM symptoms WHERE id = ?", id)
	var s Symptom
	var ts string
	if err := row.Scan(&s.ID, &s.Description, &ts, &s.Note); err != nil {
		http.Error(w, "symptom ikke funnet", http.StatusNotFound)
		return
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		http.Error(w, "ugyldig tidspunkt", http.StatusInternalServerError)
		return
	}
	s.Timestamp = t
	s.DisplayTime = t.Format("2006-01-02 15:04")
	s.InputTime = t.Local().Format("2006-01-02T15:04")
	data := struct {
		SymptomOptions []string
		Symptom        Symptom
	}{
		SymptomOptions: []string{"Hodepine", "Kvalme", "Tretthet"},
		Symptom:        s,
	}
	if err := templates.ExecuteTemplate(w, "edit_symptom.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// updateSymptomHandler processes the symptom update form.
func updateSymptomHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	id := r.FormValue("id")
	description := r.FormValue("description")
	timestampStr := r.FormValue("timestamp")
	note := r.FormValue("note")
	t, err := time.Parse("2006-01-02T15:04", timestampStr)
	if err != nil {
		http.Error(w, "ugyldig tidspunkt", http.StatusBadRequest)
		return
	}
	_, err = db.Exec("UPDATE symptoms SET description = ?, timestamp = ?, note = ? WHERE id = ?", description, t.Format(time.RFC3339), note, id)
	if err != nil {
		http.Error(w, "feil ved oppdatering", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// deleteSymptomHandler deletes a symptom entry.
func deleteSymptomHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	id := r.FormValue("id")
	_, err := db.Exec("DELETE FROM symptoms WHERE id = ?", id)
	if err != nil {
		http.Error(w, "feil ved sletting", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// exportHandler exports all data as CSV or JSON.
func exportHandler(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	meals, err := getAllMeals()
	if err != nil {
		http.Error(w, "kunne ikke hente måltider", http.StatusInternalServerError)
		return
	}
	symptoms, err := getAllSymptoms()
	if err != nil {
		http.Error(w, "kunne ikke hente symptomer", http.StatusInternalServerError)
		return
	}
	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="export.json"`)
		data := struct {
			Meals    []Meal    `json:"meals"`
			Symptoms []Symptom `json:"symptoms"`
		}{meals, symptoms}
		if err := json.NewEncoder(w).Encode(data); err != nil {
			http.Error(w, "feil ved eksport", http.StatusInternalServerError)
		}
	default:
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="export.csv"`)
		writer := csv.NewWriter(w)
		defer writer.Flush()
		writer.Write([]string{"type", "id", "value", "timestamp", "note"})
		for _, m := range meals {
			writer.Write([]string{"meal", strconv.Itoa(m.ID), m.Items, m.Timestamp.Format(time.RFC3339), m.Note})
		}
		for _, s := range symptoms {
			writer.Write([]string{"symptom", strconv.Itoa(s.ID), s.Description, s.Timestamp.Format(time.RFC3339), s.Note})
		}
	}
}
