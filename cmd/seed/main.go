package main

import (
	"fmt"
	"log"
	"os"

	"dgsmgt/internal/auth"
	"dgsmgt/internal/db"
	"dgsmgt/internal/models"

	"gorm.io/gorm"
)

var (
	osExit   = os.Exit
	database *gorm.DB
)

func main() {
	if err := Run(); err != nil {
		log.Printf("Error seeding database: %v", err)
		osExit(1)
	}
}

func Run() error {
	// The runtime is Postgres-only — no SQLite fallback.
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL must be set to a PostgreSQL DSN (e.g. host=localhost user=dgsmgt password=... dbname=dgsmgt)")
	}

	var err error
	database, err = db.InitDB(dsn)
	if err != nil {
		return err
	}

	log.Println("Seeding database...")
	seedUsers()
	seedServers()
	assignAccess()

	log.Println("Seeding complete!")
	return nil
}

func seedUsers() {
	users := []models.User{
		{Username: "admin", IsAdmin: true},
		{Username: "user1", IsAdmin: false},
		{Username: "user2", IsAdmin: false},
	}

	for i := range users {
		hashedPass, _ := auth.HashPassword("password123")
		users[i].PasswordHash = hashedPass
		if err := database.Where(models.User{Username: users[i].Username}).FirstOrCreate(&users[i]).Error; err != nil {
			log.Printf("Failed to create user %s: %v", users[i].Username, err)
		}
	}
}

func seedServers() {
	servers := []models.Server{
		{
			Name:        "Soulmask Alpha",
			ContainerID: "test-container-1",
			Image:       "soulmask-image:latest",
			ConfigJSON:  `{"ports": ["8777:8777/udp"], "env": ["SERVER_NAME=Alpha"]}`,
		},
		{
			Name:        "Soulmask Beta",
			ContainerID: "test-container-2",
			Image:       "soulmask-image:latest",
			ConfigJSON:  `{"ports": ["8778:8777/udp"], "env": ["SERVER_NAME=Beta"]}`,
		},
		{
			Name:        "Soulmask Gamma",
			ContainerID: "test-container-3",
			Image:       "soulmask-image:latest",
			ConfigJSON:  `{"ports": ["8779:8777/udp"], "env": ["SERVER_NAME=Gamma"]}`,
		},
	}

	for i := range servers {
		if err := database.Where(models.Server{Name: servers[i].Name}).FirstOrCreate(&servers[i]).Error; err != nil {
			log.Printf("Failed to create server %s: %v", servers[i].Name, err)
		}
	}
}

func assignAccess() {
	var admin, u1, u2 models.User
	database.Where("username = ?", "admin").First(&admin)
	database.Where("username = ?", "user1").First(&u1)
	database.Where("username = ?", "user2").First(&u2)

	var s []models.Server
	database.Find(&s)
	if len(s) < 3 {
		return
	}

	if admin.ID != 0 {
		for _, server := range s {
			err := database.Model(&admin).Association("Servers").Append(&server)
			if err != nil {
				log.Printf("Failed to assign server %s to admin: %v", server.Name, err)
			}
		}
	}

	if u1.ID != 0 {
		err := database.Model(&u1).Association("Servers").Append(&s[0])
		if err != nil {
			log.Printf("Failed to assign server to user1: %v", err)
		}
	}

	if u2.ID != 0 {
		err := database.Model(&u2).Association("Servers").Append(&s[1], &s[2])
		if err != nil {
			log.Printf("Failed to assign servers to user2: %v", err)
		}
	}
}
