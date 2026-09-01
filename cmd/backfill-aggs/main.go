// Command backfill-aggs replays historical orders through the exact same
// service.OnOrderPlaced path the live order.placed consumer uses — so a
// backfilled agg_restaurant_day/agg_branch_day/agg_product_day/
// agg_platform_day row can never drift from what the live event would have
// produced for the same order. See docs/implementation-plan.md Phase 10 and
// docs/ai-prompts.md's "Backfill command" prompt for the design reasoning.
//
// Historical order data comes from order-service's internal
// GET /api/internal/orders/history endpoint (one region+year at a time,
// cursor-paginated) — this service never reads order-service's Postgres
// directly (CLAUDE.md §13, "out of scope"). The response items are the
// field-for-field shape of a live order.placed event's payload
// (buildOrderPlacedPayload in order-service's order.service.ts), so there is
// no second payload shape to keep in sync with the live path.
//
// Usage:
//
//	go run ./cmd/backfill-aggs -region eg -year 2025
//	go run ./cmd/backfill-aggs -region eg -year 2025 -dry-run
//
// Idempotency: each order is deduped through the same event_ids collection
// the live consumer uses, keyed "backfill:order.placed:<orderId>" — safe to
// re-run this command for the same region/year, it will skip orders already
// applied. That dedupe key is independent of the live consumer's own
// eventId, so it does NOT protect against double-counting an order the live
// consumer has already processed from RabbitMQ — only backfill historical
// dates the live consumer never saw (e.g. before this service was deployed).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"time"

	mongodriver "go.mongodb.org/mongo-driver/mongo"

	"analytics-service/app/analytics"
	"analytics-service/app/analytics/repository"
	"analytics-service/app/analytics/service"
	"analytics-service/lib/config"
	"analytics-service/lib/logger"
	"analytics-service/pkg/httpclient"
	appmongo "analytics-service/pkg/mongo"
)

// orderHistoryItem mirrors order-service's OrderHistoryResponseDTO
// (order.response.dto.ts) — which is itself the same field shape as
// buildOrderPlacedPayload's live order.placed payload. Deliberately reused
// verbatim rather than a bespoke shape, for the same reason.
type orderHistoryItem struct {
	OrderID      string                 `json:"orderId"`
	RestaurantID int64                  `json:"restaurantId"`
	BranchID     int64                  `json:"branchId"`
	Total        int64                  `json:"total"`
	Currency     string                 `json:"currency"`
	Items        []orderHistoryLineItem `json:"items"`
	PlacedAt     string                 `json:"placedAt"`
}

type orderHistoryLineItem struct {
	ProductID int64 `json:"productId"`
	Quantity  int64 `json:"quantity"`
	LineTotal int64 `json:"lineTotal"`
}

type orderHistoryPage struct {
	Success bool                 `json:"success"`
	Data    []orderHistoryItem   `json:"data"`
	Meta    orderHistoryPageMeta `json:"meta"`
}

type orderHistoryPageMeta struct {
	NextCursor *string `json:"nextCursor"`
	HasMore    bool    `json:"hasMore"`
	Count      int     `json:"count"`
}

func main() {
	region := flag.String("region", "", "region/shard code, e.g. eg (required)")
	year := flag.Int("year", 0, "calendar year to backfill, e.g. 2025 (required)")
	limit := flag.Int("limit", 100, "page size (order-service caps this at 100)")
	dryRun := flag.Bool("dry-run", false, "fetch and report counts only; no Mongo writes")
	flag.Parse()

	if *region == "" || *year == 0 {
		fmt.Fprintln(os.Stderr, "usage: backfill-aggs -region <region> -year <year> [-limit N] [-dry-run]")
		os.Exit(2)
	}

	cfg := config.Load()
	log := logger.New(slog.LevelInfo)
	ctx := context.Background()

	if cfg.OrderService.InternalAPIKey == "" {
		fatal(fmt.Errorf("ORDER_SERVICE_INTERNAL_API_KEY is not set"))
	}

	var (
		mongoClient      *mongodriver.Client
		analyticsService *service.Service
		eventIDs         *repository.EventIDsRepo
	)
	if !*dryRun {
		client, db, err := appmongo.Connect(ctx, appmongo.Config{
			URI:            cfg.Mongo.URI,
			Database:       cfg.Mongo.Database,
			ConnectTimeout: cfg.Mongo.ConnectTimeout,
		})
		if err != nil {
			fatal(fmt.Errorf("mongo connect: %w", err))
		}
		mongoClient = client

		if err := repository.EnsureIndexes(ctx, db, cfg.EventDedupeTTL, cfg.OrderContextTTL); err != nil {
			fatal(fmt.Errorf("ensure indexes: %w", err))
		}

		restaurantDayRepo := repository.NewRestaurantDayRepo(db)
		branchDayRepo := repository.NewBranchDayRepo(db)
		productDayRepo := repository.NewProductDayRepo(db)
		platformDayRepo := repository.NewPlatformDayRepo(db)
		orderContextRepo := repository.NewOrderContextRepo(db)
		eventIDs = repository.NewEventIDsRepo(db)

		analyticsService = service.New(restaurantDayRepo, branchDayRepo, productDayRepo, platformDayRepo, orderContextRepo)
	}
	defer func() {
		if mongoClient != nil {
			_ = appmongo.Disconnect(ctx, mongoClient)
		}
	}()

	httpClient := httpclient.New(httpclient.Config{Timeout: cfg.Core.HTTPTimeout, MaxRetries: 2})

	var (
		cursor                                      string
		pages, fetched, applied, skippedDup, failed int
	)

	for {
		page, err := fetchOrderHistoryPage(ctx, httpClient, cfg, *region, *year, *limit, cursor)
		if err != nil {
			fatal(fmt.Errorf("fetch page %d: %w", pages+1, err))
		}
		pages++
		fetched += len(page.Data)

		for _, o := range page.Data {
			if *dryRun {
				log.Info("dry-run: would backfill order", "orderId", o.OrderID, "restaurantId", o.RestaurantID, "branchId", o.BranchID, "total", o.Total, "currency", o.Currency, "placedAt", o.PlacedAt)
				continue
			}

			dedupeKey := "backfill:order.placed:" + o.OrderID
			fresh, err := eventIDs.MarkSeen(ctx, dedupeKey)
			if err != nil {
				fatal(fmt.Errorf("dedupe order %s: %w", o.OrderID, err))
			}
			if !fresh {
				skippedDup++
				continue
			}

			placedAt, err := time.Parse(time.RFC3339Nano, o.PlacedAt)
			if err != nil {
				log.Error("skipping order: unparseable placedAt", "orderId", o.OrderID, "placedAt", o.PlacedAt, "error", err.Error())
				_ = eventIDs.Unmark(ctx, dedupeKey)
				failed++
				continue
			}

			items := make([]analytics.OrderPlacedItem, 0, len(o.Items))
			for _, it := range o.Items {
				items = append(items, analytics.OrderPlacedItem{
					ProductID:      it.ProductID,
					Quantity:       it.Quantity,
					LineTotalMinor: it.LineTotal,
				})
			}

			if err := analyticsService.OnOrderPlaced(ctx, analytics.OnOrderPlacedInput{
				OrderID:      o.OrderID,
				RestaurantID: o.RestaurantID,
				BranchID:     o.BranchID,
				Currency:     o.Currency,
				TotalMinor:   o.Total,
				PlacedAt:     placedAt,
				Items:        items,
			}); err != nil {
				log.Error("backfill write failed", "orderId", o.OrderID, "error", err.Error())
				_ = eventIDs.Unmark(ctx, dedupeKey)
				failed++
				continue
			}
			applied++
		}

		if !page.Meta.HasMore || page.Meta.NextCursor == nil {
			break
		}
		cursor = *page.Meta.NextCursor
	}

	log.Info("backfill complete",
		"region", *region, "year", *year, "dryRun", *dryRun,
		"pages", pages, "fetched", fetched, "applied", applied,
		"skippedAlreadyBackfilled", skippedDup, "failed", failed)

	if failed > 0 {
		os.Exit(1)
	}
}

func fetchOrderHistoryPage(ctx context.Context, c *httpclient.Client, cfg *config.Config, region string, year, limit int, cursor string) (*orderHistoryPage, error) {
	q := url.Values{}
	q.Set("region", region)
	q.Set("year", fmt.Sprintf("%d", year))
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("sortBy", "createdAt")
	q.Set("sortOrder", "asc")
	if cursor != "" {
		q.Set("cursor", cursor)
	}

	var page orderHistoryPage
	err := c.Do(ctx, httpclient.Request{
		Method:  "GET",
		URL:     cfg.OrderService.BaseURL + "/api/internal/orders/history?" + q.Encode(),
		Headers: map[string]string{"api-key": cfg.OrderService.InternalAPIKey},
	}, &page)
	if err != nil {
		return nil, err
	}
	if !page.Success {
		return nil, fmt.Errorf("order-service returned success=false")
	}
	return &page, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "backfill-aggs:", err)
	os.Exit(1)
}
