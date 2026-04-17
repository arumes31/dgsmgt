package db

import (
	"dgsmgt/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Auto-migrate the models
	err = db.AutoMigrate(&models.User{}, &models.Server{}, &models.UserServer{})
	if err != nil {
		return nil, err
	}

	DB = db
	return db, nil
}
