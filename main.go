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
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	// Time format constants
	timestampFormat = "2006-01-02T15:04"
	dateFormat      = "2006-01-02"
	displayFormat   = "2006-01-02 15:04"

	// Analysis constants
	defaultBinSizeMinutes = 15.0
	defaultTauMinutes     = 10.0
	defaultMaxLagHours    = 12
	defaultLookAheadDays  = 7
	defaultAnalysisDays   = 14
	defaultTimeSeriesDays = 30
)

// parseTimestamp parses a timestamp string in the format "2006-01-02T15:04"

// queryMealTimestamps retrieves meal timestamps within a date range
func queryMealTimestamps(start, end string) ([]time.Time, error) {
	rows, err := db.Query(
		"SELECT timestamp FROM meals WHERE DATE(timestamp) BETWEEN ? AND ? ORDER BY timestamp ASC", start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var times []time.Time
	for rows.Next() {
		var ts string
		if err := rows.Scan(&ts); err != nil {
			return nil, err
		}
		t, err := parseRFC3339(ts)
		if err != nil {
			continue
		}
		times = append(times, t)
	}
	return times, nil
}

// querySymptomTimestamps retrieves symptom timestamps within a date range
func querySymptomTimestamps(start, end string) ([]time.Time, error) {
	rows, err := db.Query(
		"SELECT timestamp FROM symptoms WHERE DATE(timestamp) BETWEEN ? AND ? ORDER BY timestamp ASC", start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var times []time.Time
	for rows.Next() {
		var ts string
		if err := rows.Scan(&ts); err != nil {
			return nil, err
		}
		t, err := parseRFC3339(ts)
		if err != nil {
			continue
		}
		times = append(times, t)
	}
	return times, nil
}

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
		Start: now.AddDate(0, 0, -defaultAnalysisDays).Format(dateFormat),
		End:   now.Format(dateFormat),
	}
	if err := templates.ExecuteTemplate(w, "crosscorr.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// crossCorrDataHandler returns JSON data for a plot showing the distribution of
// tiden (i dager) fra hvert måltid til neste symptom etterpå i valgt periode.
func crossCorrDataHandler(w http.ResponseWriter, r *http.Request) {
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" || end == "" {
		http.Error(w, "start og end må spesifiseres", http.StatusBadRequest)
		return
	}
	_, err := parseDateOnly(start)
	if err != nil {
		http.Error(w, "ugyldig startdato", http.StatusBadRequest)
		return
	}
	_, err = parseDateOnly(end)
	if err != nil {
		http.Error(w, "ugyldig sluttdato", http.StatusBadRequest)
		return
	}

	// Hent alle måltider og symptomer i perioden, sortert stigende
	mealTimes, err := queryMealTimestamps(start, end)
	if err != nil {
		http.Error(w, "kunne ikke hente måltider", http.StatusInternalServerError)
		return
	}

	sympTimes, err := querySymptomTimestamps(start, end)
	if err != nil {
		http.Error(w, "kunne ikke hente symptomer", http.StatusInternalServerError)
		return
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
	binSize := defaultBinSizeMinutes // minutter
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
	http.HandleFunc("/crosscorr", crossCorrPageHandler)
	http.HandleFunc("/crosscorr/data", crossCorrDataHandler)
	http.HandleFunc("/timeseries", timeSeriesPageHandler)
	http.HandleFunc("/timeseries/data", timeSeriesDataHandler)

	// API-endpoint for registrering av måltid
	http.HandleFunc("/api/meal", apiMealHandler)

	log.Printf("Server starting on :%d\n", *port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil); err != nil {
		log.Fatalf("server failed: %v", err)
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
		t, err := parseRFC3339(ts)
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
	extendedEnd := endDate.AddDate(0, 0, defaultLookAheadDays).Format(dateFormat) // Look ahead for symptoms

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
		t, err := parseRFC3339(ts)
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
			MealTimestamp: meal.Timestamp.Format(displayFormat),
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

// (See migrate.go)

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

	t, err := parseTimestamp(timestampStr)
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

	t, err := parseTimestamp(timestampStr)
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
	t, err := parseRFC3339(ts)
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
	t, err := parseTimestamp(timestampStr)
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
	t, err := parseRFC3339(ts)
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
	t, err := parseTimestamp(timestampStr)
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

func apiMealHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "kun POST er støttet", http.StatusMethodNotAllowed)
		return
	}
	type MealInput struct {
		Items     string `json:"items"`
		Timestamp string `json:"timestamp"`
		Note      string `json:"note"`
	}
	var input MealInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "ugyldig JSON", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(input.Items) == "" || strings.TrimSpace(input.Timestamp) == "" {
		http.Error(w, "items og timestamp må oppgis", http.StatusBadRequest)
		return
	}
	t, err := parseTimestamp(input.Timestamp)
	if err != nil {
		http.Error(w, "ugyldig timestamp-format, bruk 2006-01-02T15:04", http.StatusBadRequest)
		return
	}
	res, err := db.Exec("INSERT INTO meals (items, timestamp, note) VALUES (?, ?, ?)", input.Items, t.Format(time.RFC3339), input.Note)
	if err != nil {
		http.Error(w, "feil ved lagring", http.StatusInternalServerError)
		return
	}
	id, _ := res.LastInsertId()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
		ID     int64  `json:"id"`
	}{
		Status: "ok",
		ID:     id,
	})
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

// timeSeriesPageHandler displays the time series visualization page.
func timeSeriesPageHandler(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	data := struct{ Start, End string }{
		Start: now.AddDate(0, 0, -defaultTimeSeriesDays).Format(dateFormat),
		End:   now.Format(dateFormat),
	}
	if err := templates.ExecuteTemplate(w, "timeseries.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// TimeSeriesPoint represents a single point in the time series
type TimeSeriesPoint struct {
	Time  string  `json:"time"`
	Value float64 `json:"value"`
}

// TimeSeriesData represents the complete time series data for visualization
type TimeSeriesData struct {
	MealSeries    []TimeSeriesPoint `json:"meal_series"`
	SymptomSeries []TimeSeriesPoint `json:"symptom_series"`
}

// DetailedTimeSeriesData represents time series data broken down by individual types
type DetailedTimeSeriesData struct {
	MealSeriesByType    map[string][]TimeSeriesPoint `json:"meal_series_by_type"`
	SymptomSeriesByType map[string][]TimeSeriesPoint `json:"symptom_series_by_type"`
}

// lowPassFilter applies a first-order low-pass filter to a time series.
// y[n] = alpha * x[n] + (1-alpha) * y[n-1]
// alpha = dt / (tau + dt)
// tau: time constant in minutes
// dt: time resolution in minutes (here always 1)
// (See analysis.go)

// crossCorrelation computes the cross-correlation between two binary time series (meal/symptom).
// Returns a slice of correlation values for lags from -maxLag to +maxLag.
// (See analysis.go)

// timeSeriesDataHandler returns JSON data for cross-correlation visualization
func timeSeriesDataHandler(w http.ResponseWriter, r *http.Request) {
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	tauStr := r.URL.Query().Get("tau")
	if start == "" || end == "" {
		http.Error(w, "start og end må spesifiseres", http.StatusBadRequest)
		return
	}
	tau := defaultTauMinutes // default tau in minutes
	if tauStr != "" {
		if parsed, err := strconv.ParseFloat(tauStr, 64); err == nil && parsed > 0 {
			tau = parsed
		}
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

	// Get all meals with their items in the date range
	mealRows, err := db.Query(
		"SELECT timestamp, items FROM meals WHERE DATE(timestamp) BETWEEN ? AND ? ORDER BY timestamp ASC", start, end)
	if err != nil {
		http.Error(w, "kunne ikke hente måltider", http.StatusInternalServerError)
		return
	}
	defer mealRows.Close()

	mealsByType := make(map[string][]time.Time)
	for mealRows.Next() {
		var ts, items string
		if err := mealRows.Scan(&ts, &items); err != nil {
			http.Error(w, "feil ved scanning", http.StatusInternalServerError)
			return
		}
		t, err := parseRFC3339(ts)
		if err != nil {
			continue
		}
		// Split items by comma and create separate entries for each
		itemList := strings.Split(items, ",")
		for _, item := range itemList {
			item = strings.TrimSpace(item)
			if item != "" {
				mealsByType[item] = append(mealsByType[item], t)
			}
		}
	}

	// Get all symptoms with their descriptions in the date range
	symptomRows, err := db.Query(
		"SELECT timestamp, description FROM symptoms WHERE DATE(timestamp) BETWEEN ? AND ? ORDER BY timestamp ASC", start, end)
	if err != nil {
		http.Error(w, "kunne ikke hente symptomer", http.StatusInternalServerError)
		return
	}
	defer symptomRows.Close()

	symptomsByType := make(map[string][]time.Time)
	for symptomRows.Next() {
		var ts, description string
		if err := symptomRows.Scan(&ts, &description); err != nil {
			http.Error(w, "feil ved scanning", http.StatusInternalServerError)
			return
		}
		t, err := parseRFC3339(ts)
		if err != nil {
			continue
		}
		symptomsByType[description] = append(symptomsByType[description], t)
	}

	// Create maps for quick lookup of event times by type (rounded to minute)
	mealMinutesByType := make(map[string]map[string]bool)
	for mealType, times := range mealsByType {
		mealMinutesByType[mealType] = make(map[string]bool)
		for _, t := range times {
			localTime := t.Local()
			minuteKey := localTime.Format("2006-01-02 15:04")
			mealMinutesByType[mealType][minuteKey] = true
		}
	}

	symptomMinutesByType := make(map[string]map[string]bool)
	for symptomType, times := range symptomsByType {
		symptomMinutesByType[symptomType] = make(map[string]bool)
		for _, t := range times {
			localTime := t.Local()
			minuteKey := localTime.Format("2006-01-02 15:04")
			symptomMinutesByType[symptomType][minuteKey] = true
		}
	}

	// Generate time series for each minute in the date range
	mealRawSeries := make(map[string][]int)
	symptomRawSeries := make(map[string][]int)

	current := startDate
	for !current.After(endDate) {
		for hour := 0; hour < 24; hour++ {
			for minute := 0; minute < 60; minute++ {
				timePoint := time.Date(current.Year(), current.Month(), current.Day(), hour, minute, 0, 0, current.Location())
				timeStr := timePoint.Format("2006-01-02 15:04")

				for mealType := range mealsByType {
					if mealRawSeries[mealType] == nil {
						mealRawSeries[mealType] = []int{}
					}
					value := 0
					if mealMinutesByType[mealType][timeStr] {
						value = 1
					}
					mealRawSeries[mealType] = append(mealRawSeries[mealType], value)
				}

				for symptomType := range symptomsByType {
					if symptomRawSeries[symptomType] == nil {
						symptomRawSeries[symptomType] = []int{}
					}
					value := 0
					if symptomMinutesByType[symptomType][timeStr] {
						value = 1
					}
					symptomRawSeries[symptomType] = append(symptomRawSeries[symptomType], value)
				}
			}
		}
		current = current.AddDate(0, 0, 1)
	}

	// Filtrer seriene
	mealFiltered := make(map[string][]float64)
	symptomFiltered := make(map[string][]float64)
	for mealType, raw := range mealRawSeries {
		mealFiltered[mealType] = lowPassFilter(raw, tau)
	}
	for symptomType, raw := range symptomRawSeries {
		symptomFiltered[symptomType] = lowPassFilter(raw, tau)
	}

	// Krysskorrelasjon mellom hver måltidstype og symptomtype
	maxLag := defaultMaxLagHours * 60 // convert hours to minutes
	type CrossCorrResult struct {
		MealType    string    `json:"meal_type"`
		SymptomType string    `json:"symptom_type"`
		Lags        []int     `json:"lags"`
		Corr        []float64 `json:"corr"`
	}
	var results []CrossCorrResult
	for mealType, mealSeries := range mealFiltered {
		for symptomType, symptomSeries := range symptomFiltered {
			// Krysskorrelasjon
			lags, corr := crossCorrelation(mealSeries, symptomSeries, maxLag)
			results = append(results, CrossCorrResult{
				MealType:    mealType,
				SymptomType: symptomType,
				Lags:        lags,
				Corr:        corr,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		http.Error(w, "feil ved encoding av JSON", http.StatusInternalServerError)
		return
	}
}
