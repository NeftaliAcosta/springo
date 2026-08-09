package migrations

import (
	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/output/persistence"
	"github.com/NeftaliAcosta/springo/framework/database"

	"gorm.io/gorm"
)

func init() {
	database.RegisterMigration(database.Migration{
		Name: "20260614_000001_create_users_table",
		Up: func(dbConn *gorm.DB) error {
			return dbConn.AutoMigrate(&persistence.UserEntity{})
		},
		Down: func(dbConn *gorm.DB) error {
			return dbConn.Migrator().DropTable(&persistence.UserEntity{})
		},
	})
}
