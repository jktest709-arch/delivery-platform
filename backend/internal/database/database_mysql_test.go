package database

import (
	"os"
	"strings"
	"testing"

	"delivery-platform/backend/internal/config"
	"delivery-platform/backend/internal/model"
)

func TestMySQLMigrationAndBasicCRUD(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("MYSQL_TEST_DSN"))
	if dsn == "" {
		t.Skip("MYSQL_TEST_DSN is not configured")
	}

	db, err := Open(config.Config{DBDriver: "mysql", DBDSN: dsn})
	if err != nil {
		t.Fatalf("open mysql: %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migrate mysql: %v", err)
	}

	username := "ci-" + strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	user := model.User{
		Username:     username,
		DisplayName:  "CI integration user",
		PasswordHash: "not-used-in-this-test",
		Role:         model.RoleDeveloper,
		Status:       "enabled",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create mysql user: %v", err)
	}
	t.Cleanup(func() { db.Delete(&model.User{}, user.ID) })

	var found model.User
	if err := db.Where("username = ?", username).First(&found).Error; err != nil {
		t.Fatalf("read mysql user: %v", err)
	}
}
