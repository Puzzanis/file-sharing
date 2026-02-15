package db

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite" // Драйвер SQLite
)

var DB *sql.DB

func InitDB() {
	var err error
	// Открываем файл базы данных (создастся сам)
	DB, err = sql.Open("sqlite", "./sharing.db")
	if err != nil {
		log.Fatal(err)
	}

	// Создаем таблицу истории передач
	query := `
	CREATE TABLE IF NOT EXISTS transfer_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		sender TEXT,
		receiver TEXT,
		file_name TEXT,
		file_size INTEGER,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	_, err = DB.Exec(query)
	if err != nil {
		log.Fatal(err)
	}
}

// Функция для сохранения записи о передаче
func LogTransfer(sender, receiver, fileName string, fileSize int64) {
	query := `INSERT INTO transfer_history (sender, receiver, file_name, file_size) VALUES (?, ?, ?, ?)`
	_, err := DB.Exec(query, sender, receiver, fileName, fileSize)
	if err != nil {
		log.Printf("Ошибка записи в БД: %v", err)
	}
}

func PrintStats() {
	rows, err := DB.Query("SELECT sender, receiver, file_name, file_size, timestamp FROM transfer_history ORDER BY id DESC LIMIT 5")
	if err != nil {
		log.Println("Ошибка чтения из БД:", err)
		return
	}
	defer rows.Close()

	fmt.Println("\n--- Последние 5 передач в базе ---")
	for rows.Next() {
		var s, r, f, ts string
		var size int64
		rows.Scan(&s, &r, &f, &size, &ts)
		fmt.Printf("[%s] %s -> %s | %s (%d bytes)\n", ts, s, r, f, size)
	}
	fmt.Println("----------------------------------")
}
