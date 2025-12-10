package database

import (
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/zesbe/lumina-ai/internal/models"
)

func Connect(databaseURL string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := migrate(db); err != nil {
		return nil, err
	}

	if err := seedPlans(db); err != nil {
		log.Printf("Warning: Failed to seed plans: %v", err)
	}

	return db, nil
}

func migrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.User{},
		&models.Generation{},
		&models.Plan{},
		&models.Subscription{},
		&models.CreditTransaction{},
	)
}

func seedPlans(db *gorm.DB) error {
	for _, plan := range models.DefaultPlans {
		var existing models.Plan
		if err := db.Where("name = ?", plan.Name).First(&existing).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				if err := db.Create(&plan).Error; err != nil {
					return err
				}
				log.Printf("Created plan: %s", plan.Name)
			}
		}
	}
	return nil
}
