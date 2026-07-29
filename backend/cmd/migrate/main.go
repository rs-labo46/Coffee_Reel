package main

import (
	"coffee-reel/db"
	"coffee-reel/migrate"
	"log"
	"os"
)

func main() {
	postgresDB, err := db.NewDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}

	defer func() {
		if err := db.CloseDB(postgresDB); err != nil {
			log.Println(err)
		}
	}()

	if err := migrate.MigrateUsers(postgresDB); err != nil {
		log.Fatal(err)
	}

	if err := migrate.MigrateRefreshTokens(postgresDB); err != nil {
		log.Fatal(err)
	}

	if err := migrate.MigrateAdminAuditLogs(postgresDB); err != nil {
		log.Fatal(err)
	}

	if err := migrate.MigrateVideos(postgresDB); err != nil {
		log.Fatal(err)
	}
	if err := migrate.MigrateVideoSourceMetas(postgresDB); err != nil {
		log.Fatal(err)
	}
	if err := migrate.MigrateVideoOutputMetas(postgresDB); err != nil {
		log.Fatal(err)
	}
	if err := migrate.MigrateVideoProcessingJobs(postgresDB); err != nil {
		log.Fatal(err)
	}
	if err := migrate.MigrateSavedVideos(postgresDB); err != nil {
		log.Fatal(err)
	}
	if err := migrate.MigrateIdempotencyRecords(postgresDB); err != nil {
		log.Fatal(err)
	}
	if err := migrate.MigrateStorageCleanupJobs(postgresDB); err != nil {
		log.Fatal(err)
	}
	log.Println("migration completed")
}
