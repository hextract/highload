package store

import (
	"context"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Catalog struct {
	pool *pgxpool.Pool
}

func NewCatalog(pool *pgxpool.Pool) *Catalog {
	return &Catalog{pool: pool}
}

func (c *Catalog) Ping(ctx context.Context) error {
	return c.pool.Ping(ctx)
}

func (c *Catalog) SearchRestaurants(ctx context.Context, p SearchParams) ([]RestaurantRow, error) {
	rows, err := c.pool.Query(ctx, `
		SELECT id, name, cuisine_type, rating, avg_price, lat, lon, delivery_time_min, image_url, is_active,
			(6371000 * acos(LEAST(1.0, GREATEST(-1.0,
				cos(radians($1::float8)) * cos(radians(lat::float8)) * cos(radians(lon::float8) - radians($2::float8))
				+ sin(radians($1::float8)) * sin(radians(lat::float8))
			)))) AS distance_m
		FROM restaurants
		WHERE is_active = true
		  AND ($3::text = '' OR cuisine_type = $3)
		  AND ($4::text = '' OR name ILIKE '%' || $4 || '%')
	`, p.Lat, p.Lon, p.Cuisine, p.Query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []RestaurantRow
	for rows.Next() {
		var rr RestaurantRow
		if err := rows.Scan(&rr.ID, &rr.Name, &rr.CuisineType, &rr.Rating, &rr.AvgPrice, &rr.Lat, &rr.Lon,
			&rr.DeliveryTimeMin, &rr.ImageURL, &rr.IsOpen, &rr.DistanceM); err != nil {
			return nil, err
		}
		if rr.DistanceM <= float64(p.Radius) {
			list = append(list, rr)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	switch p.Sort {
	case "rating":
		sort.Slice(list, func(i, j int) bool { return list[i].Rating > list[j].Rating })
	case "price":
		sort.Slice(list, func(i, j int) bool { return list[i].AvgPrice < list[j].AvgPrice })
	default:
		sort.Slice(list, func(i, j int) bool { return list[i].DistanceM < list[j].DistanceM })
	}
	return list, nil
}

func (c *Catalog) RestaurantActive(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := c.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM restaurants WHERE id = $1 AND is_active)`, id).Scan(&exists)
	return exists, err
}

func (c *Catalog) MenuRows(ctx context.Context, restaurantID uuid.UUID) ([]MenuItemRow, error) {
	rows, err := c.pool.Query(ctx, `
		SELECT c.name, c.sort_order, i.id, i.name, i.description, i.price, i.image_url, i.is_available
		FROM menu_categories c
		JOIN menu_items i ON i.category_id = c.id
		WHERE c.restaurant_id = $1
		ORDER BY c.sort_order, i.name
	`, restaurantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MenuItemRow
	for rows.Next() {
		var r MenuItemRow
		if err := rows.Scan(&r.CategoryName, &r.SortOrder, &r.ItemID, &r.Name, &r.Description, &r.Price, &r.ImageURL, &r.Available); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
