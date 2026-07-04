package db

import (
	"dgsmgt/internal/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

// Open opens the database from a Postgres DSN. SQLite has been removed from
// the runtime; DATABASE_URL must be a Postgres DSN or URL.
var Open = func(dsn string) (gorm.Dialector, error) {
	return postgres.Open(dsn), nil
}

func InitDB(dsn string) (*gorm.DB, error) {
	dialector, err := Open(dsn)
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, err
	}

	if err := Migrate(db); err != nil {
		return nil, err
	}

	DB = db
	return db, nil
}

// Migrate runs AutoMigrate for all models.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(models.All()...)
}
