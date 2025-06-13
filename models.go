package main

import (
	"database/sql"
	"time"
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

// scanMealRow scans a database row into a Meal struct.
func scanMealRow(rows *sql.Rows) (Meal, error) {
	var m Meal
	var ts string
	if err := rows.Scan(&m.ID, &m.Items, &ts, &m.Note); err != nil {
		return m, err
	}
	t, err := parseRFC3339(ts)
	if err != nil {
		return m, err
	}
	m.Timestamp = t
	// DisplayTime is now set in main.go with a UTC string for client-side conversion
	// m.DisplayTime = t.Format(displayFormat) // This line is no longer needed
	m.InputTime = t.Local().Format(timestampFormat)
	return m, nil
}

// scanSymptomRow scans a database row into a Symptom struct.
func scanSymptomRow(rows *sql.Rows) (Symptom, error) {
	var s Symptom
	var ts string
	if err := rows.Scan(&s.ID, &s.Description, &ts, &s.Note); err != nil {
		return s, err
	}
	t, err := parseRFC3339(ts)
	if err != nil {
		return s, err
	}
	s.Timestamp = t
	// DisplayTime is now set in main.go with a UTC string for client-side conversion
	// s.DisplayTime = t.Format(displayFormat) // This line is no longer needed
	s.InputTime = t.Local().Format(timestampFormat)
	return s, nil
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
		m, err := scanMealRow(rows)
		if err != nil {
			return nil, err
		}
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
		s, err := scanSymptomRow(rows)
		if err != nil {
			return nil, err
		}
		symptoms = append(symptoms, s)
	}
	return symptoms, nil
}
