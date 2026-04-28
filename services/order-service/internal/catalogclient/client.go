package catalogclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type Client struct {
	base   string
	http   *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		base: baseURL,
		http: &http.Client{Timeout: 5 * time.Second},
	}
}

type MenuResponse struct {
	RestaurantID string `json:"restaurant_id"`
	Categories   []struct {
		Name  string `json:"name"`
		Items []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Price     int    `json:"price"`
			Available bool   `json:"is_available"`
		} `json:"items"`
	} `json:"categories"`
}

func (c *Client) FetchMenu(ctx context.Context, restaurantID uuid.UUID) (*MenuResponse, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/v1/restaurants/%s/menu", c.base, restaurantID.String()), nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, resp.StatusCode, fmt.Errorf("catalog: %s", string(b))
	}
	var m MenuResponse
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, resp.StatusCode, err
	}
	return &m, resp.StatusCode, nil
}
