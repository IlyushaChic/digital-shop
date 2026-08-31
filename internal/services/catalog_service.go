package services

import (
	"shop/internal/models"

	"github.com/jmoiron/sqlx"
)

type CatalogService struct {
	db *sqlx.DB
}

func NewCatalogService(db *sqlx.DB) *CatalogService {
	return &CatalogService{db: db}
}

type CatalogItem struct {
	models.Product
	Quantity int `db:"quantity" json:"quantity"`
}

func (s *CatalogService) GetCatalog() ([]CatalogItem, error) {
	var items []CatalogItem
	err := s.db.Select(&items, `
		SELECT p.sku, p.name, p.type, p.price, p.currency, p.image,
		       COALESCE(i.quantity, 0) AS quantity
		FROM products p
		LEFT JOIN inventory i ON p.sku = i.sku
		ORDER BY p.sku
	`)
	return items, err
}
