package main

import (
	"dgsmgt/internal/auth"
	"dgsmgt/internal/db"
	"dgsmgt/internal/models"
	"log"
	"os"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "dgsmgt.db"
	}

	database, err := db.InitDB(dsn)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	log.Println("Seeding database...")

	// Create Users
	users := []models.User{
		{
			Username: "admin",
			IsAdmin:  true,
		},
		{
			Username: "user1",
			IsAdmin:  false,
		},
		{
			Username: "user2",
			IsAdmin:  false,
		},
	}

	for i := range users {
		hashedPass, _ := auth.HashPassword("password123")
		users[i].PasswordHash = hashedPass
		
		// Use FirstOrCreate to avoid duplicates
		err := database.Where(models.User{Username: users[i].Username}).FirstOrCreate(&users[i]).Error
		if err != nil {
			log.Printf("Failed to create user %s: %v", users[i].Username, err)
		}
	}

	// Create Servers
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
		err := database.Where(models.Server{Name: servers[i].Name}).FirstOrCreate(&servers[i]).Error
		if err != nil {
			log.Printf("Failed to create server %s: %v", servers[i].Name, err)
		}
	}

	// Assign Servers to Users
	// Assign all servers to admin
	for _, server := range servers {
		database.Model(&users[0]).Association("Servers").Append(&server)
	}

	// Assign first server to user1
	database.Model(&users[1]).Association("Servers").Append(&servers[0])

	// Assign second and third server to user2
	database.Model(&users[2]).Association("Servers").Append(&servers[1], &servers[2])

	log.Println("Seeding complete!")
}
