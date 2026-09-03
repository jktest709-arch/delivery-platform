package database

import (
	"fmt"
	"os"
	"path/filepath"

	"delivery-platform/backend/internal/config"
	"delivery-platform/backend/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func Open(cfg config.Config) (*gorm.DB, error) {
	switch cfg.DBDriver {
	case "sqlite":
		if err := os.MkdirAll(filepath.Dir(cfg.DBDSN), 0o755); err != nil {
			return nil, err
		}
		return gorm.Open(sqlite.Open(cfg.DBDSN), &gorm.Config{})
	case "mysql":
		return gorm.Open(mysql.Open(cfg.DBDSN), &gorm.Config{})
	default:
		return nil, fmt.Errorf("unsupported DB_DRIVER %q", cfg.DBDriver)
	}
}

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&model.User{},
		&model.BusinessLine{},
		&model.Project{},
		&model.ProjectBusinessLine{},
		&model.ProjectDependency{},
		&model.Release{},
		&model.ReleaseProject{},
		&model.ReleasePipelineJob{},
		&model.ReleaseChange{},
		&model.ReleaseEvent{},
		&model.ReleaseOperationLock{},
	); err != nil {
		return err
	}
	return nil
}
