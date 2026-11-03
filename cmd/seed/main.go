package main

import (
	"dgsmgt/internal/auth"
	"dgsmgt/internal/db"
	"dgsmgt/internal/models"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

// seedCredentialsFile receives the generated demo password (in the working directory).
const seedCredentialsFile = "seed_credentials.txt"

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

	// A separate random password per account and run, so seeded databases never
	// share known credentials and an ordinary account's password never unlocks
	// the administrator account.
	passwords := make(map[string]string, len(users))
	for i := range users {
		password, err := auth.RandomString(12)
		if err != nil {
			log.Fatal("Failed to generate password:", err)
		}
		hashedPass, err := auth.HashPassword(password)
		if err != nil {
			log.Fatal("Failed to hash password:", err)
		}
		users[i].PasswordHash = hashedPass

		// Use FirstOrCreate to avoid duplicates; existing users keep their password
		result := database.Where(models.User{Username: users[i].Username}).FirstOrCreate(&users[i])
		if result.Error != nil {
			log.Printf("Failed to create user %s: %v", users[i].Username, result.Error)
			continue
		}
		if result.RowsAffected == 1 {
			passwords[users[i].Username] = password
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
	// Stored in a private file instead of the console so it does not end up in logs.
	// Each line names one account; share each password only with its holder.
	var content strings.Builder
	content.WriteString("Passwords for newly created demo users (one per account):\n")
	for _, u := range users {
		if password, ok := passwords[u.Username]; ok {
			fmt.Fprintf(&content, "%s: %s\n", u.Username, password)
		}
	}
	if err := os.WriteFile(seedCredentialsFile, []byte(content.String()), 0o600); err != nil {
		log.Fatal("Failed to write credentials file:", err)
	}
	log.Println("Demo user passwords written to " + seedCredentialsFile)
}
