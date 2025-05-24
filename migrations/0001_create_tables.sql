-- Initial migration: create tables for meals and symptoms
CREATE TABLE IF NOT EXISTS meals (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    items TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    note TEXT
);

CREATE TABLE IF NOT EXISTS symptoms (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    description TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    note TEXT
);