package main

import (
"log"
"os"

"github.com/beon/bin-api/internal/handler"
"github.com/beon/bin-api/internal/middleware"
"github.com/beon/bin-api/internal/repository/postgres"
"github.com/beon/bin-api/internal/service"
"github.com/gin-gonic/gin"
"github.com/jmoiron/sqlx"
"github.com/joho/godotenv"
_ "github.com/lib/pq"
)

func main() {
	_ = godotenv.Load()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/bindb?sslmode=disable"
	}

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer db.Close()

	enrich := os.Getenv("ENRICHMENT_ENABLED") != "false"

	repo := postgres.New(db)
	svc := service.New(repo, enrich)
	binH := handler.NewBINHandler(svc)

	r := gin.Default()
	r.SetTrustedProxies(nil)

	api := r.Group("/api/v1")
	{
		api.GET("/health", func(c *gin.Context) {
c.JSON(200, gin.H{"status": "ok"})
})
		api.GET("/stats", binH.Stats)
		api.GET("/bin/:number", binH.Lookup)
		api.GET("/bin/:number/validate", binH.ValidateCard)
	}

	admin := r.Group("/api/v1/admin", middleware.APIKeyAuth())
	_ = admin

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("BIN API listening on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
