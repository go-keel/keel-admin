package data

import (
	"gorm.io/gorm"

	"github.com/go-keel/keel/core/kdb"
)

type Data struct {
	DB *gorm.DB
}

func NewData(db *kdb.DB) *Data {
	return &Data{DB: db.DB}
}
