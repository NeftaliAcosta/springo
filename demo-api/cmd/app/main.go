package main

import (
	"log"
	"net/http"

	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/output/persistence"
	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/security"
	"github.com/NeftaliAcosta/springo/framework"
	"github.com/NeftaliAcosta/springo/framework/database"

	// Trigger auto-registration via init()
	_ "github.com/NeftaliAcosta/springo/demo-api/internal/application/service"
	_ "github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/input/events"
	_ "github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/input/rest"
	_ "github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/input/scheduler"
	_ "github.com/NeftaliAcosta/springo/demo-api/resources/db/migration"
)

// @title SprinGo API
// @version 1.0.0-rc1
// @description API Server for SprinGo Framework with Hexagonal Architecture.
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io

// @license.name MIT
// @license.url https://opensource.org/license/mit

// @host localhost:8080
// @BasePath /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Paste your token like this: Bearer <your_token>

func main() {
	// 1. Framework Bootstrap (Config + DB + Migrations + Router + Swagger)
	sprinGo := framework.Bootstrap(framework.Options{
		Middlewares: []func(http.Handler) http.Handler{
			security.SecurityHeaders,
			security.JwtAuthMiddleware,
		},
		DebugLogs: true,
	})

	// 1.5 Enable database auditing for persistence models
	if sprinGo.DB != nil {
		if err := database.EnableAuditing(sprinGo.DB, &persistence.UserEntity{}); err != nil {
			log.Fatalf("❌ Failed to enable database auditing: %v", err)
		}
	}

	// 2. Ignition
	sprinGo.Start()
}
