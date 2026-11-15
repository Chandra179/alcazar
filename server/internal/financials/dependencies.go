package financials

import (
	"database/sql"
)

type DependenciesConfig struct {
	DB *sql.DB
}

type dependencies struct {
	db *sql.DB
}

func NewDependencies(config *DependenciesConfig) *dependencies {
	return &dependencies{db: config.DB}
}
