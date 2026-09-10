package main

import (
	"log"
	"net/http"
	"os"

	tenantchat "example.com/tenant-chat-control"
)

func main() {
	client := &tenantchat.Client{
		APIKey:     os.Getenv("INFRAI_API_KEY"),
		MaxRetries: 3,
	}
	service := tenantchat.NewTenantService(client)
	log.Println("chat admin listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", tenantchat.NewAdminHandler(service)))
}
