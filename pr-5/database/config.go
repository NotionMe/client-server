package database

import (
	"notion_pr5/entity"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func StartDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	return db, db.AutoMigrate(&entity.Incident{}, &entity.User{})
}
