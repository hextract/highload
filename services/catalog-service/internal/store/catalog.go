package store

import (
	"context"
	"errors"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrRestaurantUnavailable = errors.New("restaurant missing or inactive")

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
	radiusM := float64(p.Radius)
	rows, err := c.pool.Query(ctx, `
		SELECT id, name, cuisine_type, rating, avg_price, lat, lon, delivery_time_min, image_url, is_active,
			earth_distance(
				ll_to_earth(lat::float8, lon::float8),
				ll_to_earth($1::float8, $2::float8)
			) AS distance_m
		FROM restaurants
		WHERE is_active = true
		  AND ($3::text = '' OR cuisine_type = $3)
		  AND ($4::text = '' OR name ILIKE '%' || $4 || '%')
		  AND ll_to_earth(lat::float8, lon::float8)
		      <@ earth_box(ll_to_earth($1::float8, $2::float8), $5::float8)
		  AND earth_distance(
		      ll_to_earth(lat::float8, lon::float8),
		      ll_to_earth($1::float8, $2::float8)
		    ) <= $5::float8
	`, p.Lat, p.Lon, p.Cuisine, p.Query, radiusM)
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
		list = append(list, rr)
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

// RestaurantMenu loads menu lines in one DB round-trip (restaurant row + categories + items).
// Returns ErrRestaurantUnavailable if the restaurant is missing or inactive.
func (c *Catalog) RestaurantMenu(ctx context.Context, restaurantID uuid.UUID) ([]MenuItemRow, error) {
	rows, err := c.pool.Query(ctx, `
		SELECT r.is_active,
			c.sort_order,
			c.name,
			i.id,
			i.name,
			i.description,
			i.price,
			i.image_url,
			i.is_available
		FROM restaurants r
		LEFT JOIN menu_categories c ON c.restaurant_id = r.id AND r.is_active
		LEFT JOIN menu_items i ON i.category_id = c.id
		WHERE r.id = $1
		ORDER BY c.sort_order NULLS LAST, i.name NULLS LAST
	`, restaurantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MenuItemRow
	first := true
	for rows.Next() {
		var isActive bool
		var sort *int
		var catName *string
		var itemID *uuid.UUID
		var name, desc, img *string
		var price *int
		var avail *bool
		if err := rows.Scan(&isActive, &sort, &catName, &itemID, &name, &desc, &price, &img, &avail); err != nil {
			return nil, err
		}
		if first {
			first = false
			if !isActive {
				return nil, ErrRestaurantUnavailable
			}
		}
		if itemID == nil {
			continue
		}
		out = append(out, MenuItemRow{
			CategoryName: *catName,
			SortOrder:    *sort,
			ItemID:       *itemID,
			Name:         *name,
			Description:  stringOrEmpty(desc),
			Price:        *price,
			ImageURL:     stringOrEmpty(img),
			Available:    *avail,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if first {
		return nil, ErrRestaurantUnavailable
	}
	return out, nil
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
