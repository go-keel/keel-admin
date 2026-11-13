package data

import (
	"gorm.io/gorm"

	"github.com/go-keel/keel/core/kdb"
)

// Data is the business area's database handle; the kernel holds its own internal/core/data.Data. Business tables
// live in the business database (database.default in config.yaml), sys_* tables in the admin database
// (database.admin); both are required.
//
// Business repos always take the DB from here and never borrow the kernel's Data: sharing one handle in the types
// would weld shut the option of splitting the databases.
type Data struct {
	DB *gorm.DB
}

func NewData(db *kdb.DB) *Data { return &Data{DB: db.DB} }
