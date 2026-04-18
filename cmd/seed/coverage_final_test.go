package main

import (
	"os"
	"testing"
	"dgsmgt/internal/db"
	"dgsmgt/internal/models"
)

func TestSeed_Functions(t *testing.T) {
	os.Setenv("DATABASE_URL", ":memory:")
	defer os.Unsetenv("DATABASE_URL")
	
	var err error
	database, err = db.InitDB(":memory:")
	if err != nil { t.Fatal(err) }

	t.Run("PartialAccess", func(t *testing.T) {
		assignAccess() // hit len(s) < 3
	})

	t.Run("FullFlow", func(t *testing.T) {
		seedUsers()
		seedServers()
		assignAccess()
	})

	t.Run("seedUsersFail", func(t *testing.T) {
		_ = database.Migrator().DropTable("users")
		seedUsers()
	})
	t.Run("seedServersFail", func(t *testing.T) {
		_ = database.Migrator().DropTable("servers")
		seedServers()
	})
	t.Run("assignAccessFail", func(t *testing.T) {
		// Just ensure tables exist before dropping user_servers
		_ = database.AutoMigrate(&models.User{}, &models.Server{}, &models.UserServer{})
		seedUsers()
		seedServers()
		_ = database.Migrator().DropTable("user_servers")
		assignAccess()
	})
}
