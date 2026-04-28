package store

import "github.com/google/uuid"

type RestaurantRow struct {
	ID              uuid.UUID
	Name            string
	CuisineType     string
	Rating          float64
	AvgPrice        int
	Lat, Lon        float64
	DistanceM       float64
	DeliveryTimeMin int
	ImageURL        string
	IsOpen          bool
}

type SearchParams struct {
	Lat, Lon       float64
	Radius         int
	Page, Limit    int
	Cuisine, Query string
	Sort           string
}

type MenuItemRow struct {
	CategoryName string
	SortOrder    int
	ItemID       uuid.UUID
	Name         string
	Description  string
	Price        int
	ImageURL     string
	Available    bool
}
