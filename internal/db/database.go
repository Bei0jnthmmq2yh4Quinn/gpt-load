package db

import (
	"fmt"
	"gpt-load/internal/types"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func NewDB(configManager types.ConfigManager) (*gorm.DB, error) {
	dbConfig := configManager.GetDatabaseConfig()
	dsn := dbConfig.DSN
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_DSN is not configured")
	}

	var newLogger logger.Interface
	if configManager.GetLogConfig().Level == "debug" {
		newLogger = logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
			logger.Config{
				SlowThreshold:             time.Second, // Slow SQL threshold
				LogLevel:                  logger.Info, // Log level
				IgnoreRecordNotFoundError: true,        // Ignore ErrRecordNotFound error for logger
				Colorful:                  true,        // Disable color
			},
		)
	}

	var dialector gorm.Dialector
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		dialector = postgres.New(postgres.Config{
			DSN:                  dsn,
			PreferSimpleProtocol: true,
		})
	} else if strings.Contains(dsn, "@tcp") {
		if !strings.Contains(dsn, "parseTime") {
			if strings.Contains(dsn, "?") {
				dsn += "&parseTime=true"
			} else {
				dsn += "?parseTime=true"
			}
		}
		dialector = mysql.Open(dsn)
	} else {
		// --- MODIFICATION START ---
		// This block is modified to handle deployments on read-only filesystems
		// by using an environment variable for the data path.
		
		// Get the storage path from an environment variable.
		// On Choreo, this should be set to the writable mount path (e.g., /data/writable).
		dataPath := os.Getenv("DATA_PATH")
		
		// If DATA_PATH is set, we assume the DSN from config is just a filename
		// and we construct the full path.
		if dataPath != "" {
			dsn = filepath.Join(dataPath, dsn)
		}

		// Ensure the directory for the SQLite file exists.
		dbDir := filepath.Dir(dsn)
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			// The error message now includes the path it tried to create for easier debugging.
			return nil, fmt.Errorf("failed to create database directory at '%s': %w", dbDir, err)
		}
		dialector = sqlite.Open(dsn + "?_busy_timeout=5000")
		// --- MODIFICATION END ---
	}

	var err error
	DB, err = gorm.Open(dialector, &gorm.Config{
		Logger:      newLogger,
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}
	// Set connection pool parameters for all drivers
	sqlDB.SetMaxIdleConns(50)
	sqlDB.SetMaxOpenConns(500)
	sqlDB.SetConnMaxLifetime(time.Hour)

	return DB, nil
}
