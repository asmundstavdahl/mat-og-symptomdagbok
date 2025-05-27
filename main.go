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

/*
crossCorrDataHandler returns JSON data for a plot showing the distribution of
tiden (i dager) fra hvert måltid til neste symptom etterpå i valgt periode.
*/
func crossCorrDataHandler(w http.ResponseWriter, r *http.Request) {
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" || end == "" {
		http.Error(w, "start og end må spesifiseres", http.StatusBadRequest)
		return
	}
	layout := "2006-01-02"
	// startDate and endDate are not used, so don't declare them
	_, err := time.Parse(layout, start)
	if err != nil {
		http.Error(w, "ugyldig startdato", http.StatusBadRequest)
		return
	}
	_, err = time.Parse(layout, end)
	if err != nil {
		http.Error(w, "ugyldig sluttdato", http.StatusBadRequest)
		return
	}

	// Hent alle måltider og symptomer i perioden, sortert stigende
	mealRows, err := db.Query(
		"SELECT timestamp FROM meals WHERE DATE(timestamp) BETWEEN ? AND ? ORDER BY timestamp ASC", start, end)
	if err != nil {
		http.Error(w, "kunne ikke hente måltider", http.StatusInternalServerError)
		return
	}
	defer mealRows.Close()
	var mealTimes []time.Time
	for mealRows.Next() {
		var ts string
		if err := mealRows.Scan(&ts); err != nil {
			http.Error(w, "feil ved scanning", http.StatusInternalServerError)
			return
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		mealTimes = append(mealTimes, t)
	}

	sympRows, err := db.Query(
		"SELECT timestamp FROM symptoms WHERE DATE(timestamp) BETWEEN ? AND ? ORDER BY timestamp ASC", start, end)
	if err != nil {
		http.Error(w, "kunne ikke hente symptomer", http.StatusInternalServerError)
		return
	}
	defer sympRows.Close()
	var sympTimes []time.Time
	for sympRows.Next() {
		var ts string
		if err := sympRows.Scan(&ts); err != nil {
			http.Error(w, "feil ved scanning", http.StatusInternalServerError)
			return
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			continue
		}
		sympTimes = append(sympTimes, t)
	}

	// For hvert måltid, finn første symptom etterpå og regn ut antall minutter
	var delays []float64
	for _, meal := range mealTimes {
		minDelay := -1.0
		for _, symp := range sympTimes {
			if symp.After(meal) {
				delay := symp.Sub(meal).Minutes()
				if minDelay < 0 || delay < minDelay {
					minDelay = delay
				}
			}
		}
		if minDelay >= 0 {
			delays = append(delays, minDelay)
		}
	}

	// Bygg histogram: grupper forsinkelse i 15-minutters intervaller
	binSize := 15.0 // minutter
	hist := make(map[int]int)
	for _, d := range delays {
		bin := int(math.Floor(d / binSize))
		hist[bin]++
	}
	// Sorter bins
	var bins []int
	for k := range hist {
		bins = append(bins, k)
	}
	sort.Ints(bins)
	var counts []int
	for _, b := range bins {
		counts = append(counts, hist[b])
	}

	// Returner som JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Bins   []int `json:"bins"`
		Counts []int `json:"counts"`
	}{
		Bins:   bins,
		Counts: counts,
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
	http.HandleFunc("/report/meal-symptom-data", mealSymptomDataHandler)
	http.HandleFunc("/meal-symptom-analysis", mealSymptomAnalysisHandler)
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

// MealSymptomData represents the time difference between a meal and the next symptom
type MealSymptomData struct {
	MealID          int      `json:"meal_id"`
	MealItems       string   `json:"meal_items"`
	MealTimestamp   string   `json:"meal_timestamp"`
	NextSymptomID   *int     `json:"next_symptom_id"`
	NextSymptomDesc *string  `json:"next_symptom_desc"`
	TimeDiffHours   *float64 `json:"time_diff_hours"`
}

// mealSymptomDataHandler returns JSON data showing time differences between meals and next symptoms
func mealSymptomDataHandler(w http.ResponseWriter, r *http.Request) {
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" || end == "" {
		http.Error(w, "start og end må spesifiseres", http.StatusBadRequest)
		return
	}

	// Get all meals in the date range
	mealRows, err := db.Query(
		"SELECT id, items, timestamp FROM meals WHERE DATE(timestamp) BETWEEN ? AND ? ORDER BY timestamp",
		start, end)
	if err != nil {
		http.Error(w, "kunne ikke hente måltider", http.StatusInternalServerError)
		return
	}
	defer mealRows.Close()

	var meals []struct {
		ID        int
		Items     string
		Timestamp time.Time
	}

	for mealRows.Next() {
		var m struct {
			ID        int
			Items     string
			Timestamp time.Time
		}
		var ts string
		if err := mealRows.Scan(&m.ID, &m.Items, &ts); err != nil {
			http.Error(w, "feil ved scanning av måltider", http.StatusInternalServerError)
			return
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			http.Error(w, "ugyldig tidspunkt for måltid", http.StatusInternalServerError)
			return
		}
		m.Timestamp = t
		meals = append(meals, m)
	}

	// Get all symptoms in the date range (extended to catch symptoms after meals)
	endDate, err := time.Parse("2006-01-02", end)
	if err != nil {
		http.Error(w, "ugyldig sluttdato", http.StatusBadRequest)
		return
	}
	extendedEnd := endDate.AddDate(0, 0, 7).Format("2006-01-02") // Look 7 days ahead

	symptomRows, err := db.Query(
		"SELECT id, description, timestamp FROM symptoms WHERE DATE(timestamp) BETWEEN ? AND ? ORDER BY timestamp",
		start, extendedEnd)
	if err != nil {
		http.Error(w, "kunne ikke hente symptomer", http.StatusInternalServerError)
		return
	}
	defer symptomRows.Close()

	var symptoms []struct {
		ID          int
		Description string
		Timestamp   time.Time
	}

	for symptomRows.Next() {
		var s struct {
			ID          int
			Description string
			Timestamp   time.Time
		}
		var ts string
		if err := symptomRows.Scan(&s.ID, &s.Description, &ts); err != nil {
			http.Error(w, "feil ved scanning av symptomer", http.StatusInternalServerError)
			return
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			http.Error(w, "ugyldig tidspunkt for symptom", http.StatusInternalServerError)
			return
		}
		s.Timestamp = t
		symptoms = append(symptoms, s)
	}

	// Calculate time differences between meals and next symptoms
	var result []MealSymptomData
	for _, meal := range meals {
		data := MealSymptomData{
			MealID:        meal.ID,
			MealItems:     meal.Items,
			MealTimestamp: meal.Timestamp.Format("2006-01-02 15:04"),
		}

		// Find the next symptom after this meal
		for _, symptom := range symptoms {
			if symptom.Timestamp.After(meal.Timestamp) {
				data.NextSymptomID = &symptom.ID
				data.NextSymptomDesc = &symptom.Description
				timeDiff := symptom.Timestamp.Sub(meal.Timestamp).Hours()
				data.TimeDiffHours = &timeDiff
				break
			}
		}

		result = append(result, data)
	}

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

func mealSymptomAnalysisHandler(w http.ResponseWriter, r *http.Request) {
	// Get date range from query parameters or use defaults
	startDate := r.URL.Query().Get("start")
	endDate := r.URL.Query().Get("end")

	if startDate == "" {
		startDate = time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = time.Now().Format("2006-01-02")
	}

	data := struct {
		StartDate string
		EndDate   string
	}{
		StartDate: startDate,
		EndDate:   endDate,
	}

	tmpl, err := template.ParseFiles("templates/meal_symptom_analysis.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
