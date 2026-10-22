package main

import (
	"dgsmgt/internal/auth"
	"dgsmgt/internal/db"
	"dgsmgt/internal/models"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// Seeds demo users and servers for local development only.
func main() {
	dev := flag.Bool("dev", false, "confirm that the target database is a local development database")
	flag.Parse()
	if !*dev {
		log.Fatal("The seed command creates demo accounts and is meant for development only; re-run with -dev to confirm")
	}

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

	// A random password per run, so seeded databases never share known credentials
	password, err := auth.RandomString(12)
	if err != nil {
		log.Fatal("Failed to generate password:", err)
	}
	hashedPass, err := auth.HashPassword(password)
	if err != nil {
		log.Fatal("Failed to hash password:", err)
	}

	for i := range users {
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
	// Stored in a private file instead of the console so it does not end up in logs
	credFile := filepath.Join(filepath.Dir(dsn), "seed_credentials.txt")
	content := fmt.Sprintf("Password for newly created demo users (admin, user1, user2): %s\n", password)
	if err := os.WriteFile(credFile, []byte(content), 0o600); err != nil {
		log.Fatal("Failed to write credentials file:", err)
	}
	log.Printf("Demo user password written to %s", credFile)
}
