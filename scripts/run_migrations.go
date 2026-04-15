//go:build ignore

package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	dbUrl := os.Getenv("DATABASE_URL")
	if dbUrl == "" {
		log.Fatal("DATABASE_URL must be set")
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbUrl)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer conn.Close(ctx)

	log.Println("Connected to database successfully")

	migrationsDir := "scripts/migrations"
	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		log.Fatalf("Failed to read migrations directory: %v\n", err)
	}

	var upFiles []string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".up.sql") {
			upFiles = append(upFiles, f.Name())
		}
	}
	sort.Strings(upFiles)

	for _, f := range upFiles {
		log.Printf("Executing migration: %s\n", f)
		content, err := os.ReadFile(filepath.Join(migrationsDir, f))
		if err != nil {
			log.Fatalf("Failed to read migration file %s: %v\n", f, err)
		}

		_, err = conn.Exec(ctx, string(content))
		if err != nil {
			log.Fatalf("Failed to execute migration %s: %v\n", f, err)
		}
		log.Printf("Migration %s executed successfully\n", f)
	}

	log.Println("All migrations executed successfully")
}
