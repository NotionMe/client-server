package entity

import "time"

type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Email        string    `gorm:"uniqueIndex;not null"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"CreatedAt"`
}
