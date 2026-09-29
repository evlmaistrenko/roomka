// Command control is the roomka control server: a GraphQL API over sqlite for
// accounts, authentication and access control, with the service's own business
// logic built on top of that base. A reverse proxy (Vite in development, Caddy
// in the container) fronts it so the browser reaches it same-origin under
// /graphql.
package main

import (
	"log"

	"control/graph"
	"control/internal/accounts"
	"control/internal/config"
	"control/internal/database"
	"control/internal/events"
	"control/internal/rbac"
	"control/internal/server"
)

func main() {
	configuration := config.Load()

	store, err := database.Open(configuration.DatabasePath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer store.Close()

	// The role→permission map is declared in the schema by @grants; gqlgen never
	// calls an ENUM_VALUE directive, so it is read from the parsed schema here.
	// Load also validates every literal those directives name, which nothing else
	// in either toolchain does.
	roles, err := rbac.Load(graph.Schema(), store)
	if err != nil {
		log.Fatalf("failed to load roles from schema: %v", err)
	}

	resolver := &graph.Resolver{
		Accounts: accounts.New(configuration, store, events.NewBus(), roles),
	}

	handler, err := server.New(configuration, store, resolver)
	if err != nil {
		log.Fatalf("failed to build the HTTP handler: %v", err)
	}

	address := ":" + configuration.APIPort
	log.Printf("control server listening on %s (graphql at /graphql)", address)
	if err := server.HTTPServer(address, handler).ListenAndServe(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
