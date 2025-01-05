package util

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

//go:embed assets/database/*
var embeddedFiles embed.FS

func CreateControllerDatabase(host, user, password, provider, dbName string, port int) error {
	var dsn, migrationFile, seederFile, checkDBQuery, createDBQuery string

	// Determine DSN and file paths based on the provider
	switch provider {
	case "mysql":
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/", user, password, host, port)
		migrationFile = "assets/database/db_migration_v1.0.2.sql"
		seederFile = "assets/database/db_seeder_v1.0.2.sql"
		checkDBQuery = fmt.Sprintf("SHOW DATABASES LIKE '%s'", dbName)
		createDBQuery = fmt.Sprintf("CREATE DATABASE `%s`", dbName)
	case "postgres":
		dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s sslmode=disable", host, port, user, password)
		migrationFile = "assets/database/db_migration_postgre_v1.0.2.sql"
		seederFile = "assets/database/db_seeder_postgre_v1.0.2.sql"
		checkDBQuery = fmt.Sprintf("SELECT 1 FROM pg_database WHERE datname='%s'", dbName)
		createDBQuery = fmt.Sprintf("CREATE DATABASE %s", dbName)
	default:
		return fmt.Errorf("unsupported provider: %s", provider)
	}

	// Connect to the database server
	db, err := sql.Open(provider, dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to the %s server: %v", provider, err)
	}
	defer db.Close()

	// Check if the database exists
	rows, err := db.Query(checkDBQuery)
	if err != nil {
		return fmt.Errorf("failed to query databases: %v", err)
	}
	defer rows.Close()

	var databaseExists bool
	if rows.Next() {
		databaseExists = true
	}

	if !databaseExists {
		// Create the new database if it doesn't exist
		_, err = db.Exec(createDBQuery)
		if err != nil {
			return fmt.Errorf("failed to create database: %v", err)
		}
		log.Printf("Database %s created successfully\n", dbName)

		// Reconnect to the newly created database (Postgres requires a new connection)
		if provider == "mysql" {
			_, err = db.Exec("USE `" + dbName + "`")
		} else {
			db.Close()
			db, err = sql.Open(provider, dsn+" dbname="+dbName)
		}
		if err != nil {
			return fmt.Errorf("failed to switch to database: %v", err)
		}
		defer db.Close()

		// Run migration scripts
		if err := runSQLScripts(db, migrationFile, provider); err != nil {
			return err
		}
		log.Printf("Migration SQL executed successfully")

		// Run seeding scripts
		if err := runSQLScripts(db, seederFile, provider); err != nil {
			return err
		}
		log.Printf("Seeding SQL executed successfully")
	} else {
		// Reconnect to the existing database
		if provider == "mysql" {
			_, err = db.Exec("USE `" + dbName + "`")
		} else {
			db.Close()
			db, err = sql.Open(provider, dsn+" dbname="+dbName)
		}
		if err != nil {
			return fmt.Errorf("failed to switch to database: %v", err)
		}
		defer db.Close()

		// Run migration scripts
		if err := runSQLScripts(db, migrationFile, provider); err != nil {
			return err
		}
		log.Printf("Migration SQL executed successfully")
		log.Printf("Database %s already exists. Only migration script executed.", dbName)
	}

	return nil
}

// Helper function to execute SQL scripts
func runSQLScripts(db *sql.DB, scriptFile, provider string) error {
	scriptSQL, err := embeddedFiles.ReadFile(scriptFile)
	if err != nil {
		return fmt.Errorf("failed to read SQL file %s: %v", scriptFile, err)
	}

	statements := strings.Split(string(scriptSQL), ";")
	for _, statement := range statements {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}

		_, err := db.Exec(statement)
		if err != nil {
			if provider == "mysql" {
				// Handle MySQL specific errors
				if strings.Contains(err.Error(), "Error 1050") ||
					strings.Contains(err.Error(), "Error 1060") ||
					strings.Contains(err.Error(), "Error 1054") ||
					strings.Contains(err.Error(), "Error 1061") {
					log.Printf("Ignoring known MySQL error: %v", err)
					continue
				}
			} else if provider == "postgres" {
				// Handle PostgreSQL specific errors
				if strings.Contains(err.Error(), "duplicate_key") ||
					strings.Contains(err.Error(), "already exists") {
					log.Printf("Ignoring known PostgreSQL error: %v", err)
					continue
				}
			}
			return fmt.Errorf("failed to execute SQL statement: %v", err)
		}
	}

	return nil
}
