package util

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

//go:embed assets/database/*
var embeddedFiles embed.FS

func CreateControllerDatabase(host, user, password, provider, dbName string, port int) error {
	// Create MySQL DSN (Data Source Name)
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/", user, password, host, port)

	// Connect to MySQL server
	db, err := sql.Open(provider, dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to MySQL server: %v", err)
	}
	defer db.Close()

	// Check if the database exists
	query := fmt.Sprintf("SHOW DATABASES LIKE '%s'", dbName)
	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("failed to query databases: %v", err)
	}
	defer rows.Close()

	var databaseExists bool
	for rows.Next() {
		var databaseName string
		if err := rows.Scan(&databaseName); err != nil {
			return fmt.Errorf("failed to scan database row: %v", err)
		}
		if databaseName == dbName {
			databaseExists = true
			break
		}
	}

	if !databaseExists {
		// Create the new database if it doesn't exist
		_, err = db.Exec("CREATE DATABASE `" + dbName + "`")
		if err != nil {
			return fmt.Errorf("failed to create database: %v", err)
		}
		log.Printf("Database %s created successfully\n", dbName)

		// Switch to the newly created database
		_, err = db.Exec("USE `" + dbName + "`")
		if err != nil {
			return fmt.Errorf("failed to switch to database: %v", err)
		}

		// Read migration SQL file
		migrationSQL, err := embeddedFiles.ReadFile("assets/database/db_migration_v1.0.1.sql")
		if err != nil {
			return fmt.Errorf("failed to read migration SQL file: %v", err)
		}

		// Split SQL script into individual statements
		migrationStatements := strings.Split(string(migrationSQL), ";")

		// Execute each migration SQL statement individually
		for _, statement := range migrationStatements {
			statement = strings.TrimSpace(statement)
			if statement == "" {
				continue // Skip empty statements
			}

			// Execute the SQL statement
			_, err := db.Exec(statement)
			if err != nil {
				return fmt.Errorf("failed to execute migration SQL statement: %v", err)
			}
		}

		log.Printf("Migration SQL executed successfully")

		// Read seeding SQL file
		seedingSQL, err := embeddedFiles.ReadFile("assets/database/db_seeder_v1.0.0.sql")
		if err != nil {
			return fmt.Errorf("failed to read seeding SQL file: %v", err)
		}

		// Split SQL script into individual statements
		seedStatements := strings.Split(string(seedingSQL), ";")

		// Execute each seeding SQL statement individually
		for _, statement := range seedStatements {
			statement = strings.TrimSpace(statement)
			if statement == "" {
				continue // Skip empty statements
			}

			// Execute the SQL statement
			_, err := db.Exec(statement)
			if err != nil {
				return fmt.Errorf("failed to execute seeding SQL statement: %v", err)
			}
		}

		log.Printf("Seeding SQL executed successfully")
	} else {

		// Switch to the existing database
		_, err = db.Exec("USE `" + dbName + "`")
		if err != nil {
			return fmt.Errorf("failed to switch to database: %v", err)
		}

		// Read migration SQL file
		migrationSQL, err := embeddedFiles.ReadFile("assets/database/db_migration_v1.0.1.sql")
		if err != nil {
			return fmt.Errorf("failed to read migration SQL file: %v", err)
		}

		// Split SQL script into individual statements
		migrationStatements := strings.Split(string(migrationSQL), ";")

		// Execute each migration SQL statement individually
		for _, statement := range migrationStatements {
			statement = strings.TrimSpace(statement)
			if statement == "" {
				continue // Skip empty statements
			}

			// Execute the SQL statement
			_, err := db.Exec(statement)
			if err != nil {
				// Check for specific errors to ignore
				if strings.Contains(err.Error(), "Error 1050") ||
					strings.Contains(err.Error(), "Error 1060") ||
					strings.Contains(err.Error(), "Error 1061") {
					log.Printf("Ignoring known error: %v", err)
					continue
				}
				return fmt.Errorf("failed to execute migration SQL statement: %v", err)
			}
		}

		log.Printf("Migration SQL executed successfully")

		log.Printf("Database %s already exists. Only migration script executed.", dbName)
	}

	return nil
}
