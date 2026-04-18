package main

import (
	"dgsmgt/internal/auth"
	"dgsmgt/internal/db"
	"dgsmgt/internal/models"
	"log"
	"os"
)

var osExit = os.Exit

func main() {
	if err := Run(); err != nil {
		log.Printf("Error seeding database: %v", err)
		osExit(1)
	}
}

func Run() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "dgsmgt.db"
	}

	database, err := db.InitDB(dsn)
	if err != nil {
		return err
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
		if err := database.Model(&users[0]).Association("Servers").Append(&server); err != nil {
			log.Printf("Failed to assign server %s to admin: %v", server.Name, err)
		}
	}

	// Assign first server to user1
	if err := database.Model(&users[1]).Association("Servers").Append(&servers[0]); err != nil {
		log.Printf("Failed to assign server to user1: %v", err)
	}

	// Assign second and third server to user2
	if err := database.Model(&users[2]).Association("Servers").Append(&servers[1], &servers[2]); err != nil {
		log.Printf("Failed to assign servers to user2: %v", err)
	}

	log.Println("Seeding complete!")
	return nil
}
