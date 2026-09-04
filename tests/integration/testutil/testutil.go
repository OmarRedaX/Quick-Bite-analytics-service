// Package testutil is the shared scaffold for analytics-service's
// integration suite: a real (test-database) Mongo connection, JWT minting
// matching the wire shape lib/auth/jwt.go verifies, a fixed-answer RBAC
// cache (no HTTP fake needed — rbac.Cache takes its fetcher as a plain
// injected function), and a router built the same way lib/boot/boot.go
// builds the real one, from the same exported symbols. See
// testing-implementation-plan.md Phase 1 for why this duplicates boot.go's
// wiring instead of boot.go exporting a shared builder.
package testutil

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	golangjwt "github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/mongo"

	"analytics-service/app/analytics/controller"
	"analytics-service/app/analytics/repository"
	"analytics-service/app/analytics/service"
	"analytics-service/lib/rbac"
	appmongo "analytics-service/pkg/mongo"
)

const (
	// TestAccessSecret is the HS256 secret every minted test token is
	// signed with and every test router verifies against — arbitrary, only
	// needs to match between MintAccessToken and NewRouter within a test.
	TestAccessSecret = "test-access-secret-do-not-use-in-prod"

	eventDedupeTTL  = 7 * 24 * time.Hour
	orderContextTTL = 45 * 24 * time.Hour
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ConnectMongo connects to a real local Mongo test database (MONGO_URI /
// MONGO_TEST_DATABASE env vars, defaulting to localhost / a _test-suffixed
// name so a stray run can never touch the dev database), ensures every
// index the app declares, and registers a t.Cleanup that drops the test
// database and disconnects. Every integration test that touches data calls
// this once, directly — no shared TestMain-level connection, so tests stay
// independent of run order/parallelism.
func ConnectMongo(t *testing.T) *mongo.Database {
	t.Helper()

	uri := envOr("MONGO_URI", "mongodb://localhost:27017")
	dbName := envOr("MONGO_TEST_DATABASE", "quickbite_analytics_test")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, db, err := appmongo.Connect(ctx, appmongo.Config{
		URI:            uri,
		Database:       dbName,
		ConnectTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("testutil: connect mongo: %v", err)
	}

	if err := repository.EnsureIndexes(ctx, db, eventDedupeTTL, orderContextTTL); err != nil {
		t.Fatalf("testutil: ensure indexes: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = db.Drop(cleanupCtx)
		_ = appmongo.Disconnect(cleanupCtx, client)
	})

	return db
}

// claims mirrors lib/auth/jwt.go's unexported jwtClaims wire shape (same
// json tags) — duplicated here on purpose: this service never signs
// tokens in production, so there is no production signing code to reuse,
// only the verify side.
type claims struct {
	UserID         int64   `json:"userId"`
	Role           string  `json:"role"`
	Email          string  `json:"email"`
	RestaurantID   *int64  `json:"restaurantId,omitempty"`
	RestaurantRole string  `json:"restaurantRole,omitempty"`
	BranchIDs      []int64 `json:"branchIds,omitempty"`
	golangjwt.RegisteredClaims
}

type ClaimOption func(*claims)

func WithRestaurantRole(role string) ClaimOption {
	return func(c *claims) { c.RestaurantRole = role }
}

func WithRestaurantID(id int64) ClaimOption {
	return func(c *claims) { c.RestaurantID = &id }
}

// MintAccessToken signs a token with TestAccessSecret in the exact shape
// lib/auth.VerifyAccessToken expects. role is the top-level appcontext
// role ("system_admin" bypasses the RBAC cache entirely — see
// lib/rbac/middleware.go — "restaurant_user" is checked against
// RestaurantRole via whatever *rbac.Cache the router was built with).
func MintAccessToken(t *testing.T, userID int64, email, role string, opts ...ClaimOption) string {
	t.Helper()

	c := claims{
		UserID: userID,
		Role:   role,
		Email:  email,
		RegisteredClaims: golangjwt.RegisteredClaims{
			ExpiresAt: golangjwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	for _, opt := range opts {
		opt(&c)
	}

	token := golangjwt.NewWithClaims(golangjwt.SigningMethodHS256, c)
	signed, err := token.SignedString([]byte(TestAccessSecret))
	if err != nil {
		t.Fatalf("testutil: sign token: %v", err)
	}
	return signed
}

// FixedPermissionsCache returns an *rbac.Cache whose fetcher always
// returns perms, regardless of role — no HTTP fake needed, since
// rbac.Cache takes its fetch function as a plain injected value. Use for
// "restaurant_user"-role test requests; "system_admin" requests never
// consult the cache at all (rbac.Require's own short-circuit).
func FixedPermissionsCache(perms ...string) *rbac.Cache {
	return rbac.NewCache(5*time.Minute, func(_ context.Context, _ string) ([]string, error) {
		return perms, nil
	})
}

// NewRouter builds the real analytics router against db, wired exactly the
// way lib/boot/boot.go wires the production one (same exported
// constructors, same mount path) — see the package doc comment for why
// this duplicates boot.go's ~15 lines rather than boot.go exporting a
// shared builder.
func NewRouter(db *mongo.Database, accessSecret string, permCache *rbac.Cache) http.Handler {
	restaurantDayRepo := repository.NewRestaurantDayRepo(db)
	branchDayRepo := repository.NewBranchDayRepo(db)
	productDayRepo := repository.NewProductDayRepo(db)
	platformDayRepo := repository.NewPlatformDayRepo(db)
	orderContextRepo := repository.NewOrderContextRepo(db)

	analyticsService := service.New(restaurantDayRepo, branchDayRepo, productDayRepo, platformDayRepo, orderContextRepo)
	analyticsController := controller.New(analyticsService)

	router := chi.NewRouter()
	router.Mount("/api/v1/analytics", controller.Routes(accessSecret, permCache, analyticsController))
	return router
}

// RepoBundle exposes the same repository constructors NewRouter wires
// internally, so integration tests can seed Mongo through the real
// Apply*/Save methods (exercising the actual upsert/date-bucketing logic)
// instead of hand-writing bson documents that could drift from the schema.
type RepoBundle struct {
	RestaurantDay *repository.RestaurantDayRepo
	BranchDay     *repository.BranchDayRepo
	ProductDay    *repository.ProductDayRepo
	PlatformDay   *repository.PlatformDayRepo
	OrderContext  *repository.OrderContextRepo
	EventIDs      *repository.EventIDsRepo
}

func NewRepoBundle(db *mongo.Database) *RepoBundle {
	return &RepoBundle{
		RestaurantDay: repository.NewRestaurantDayRepo(db),
		BranchDay:     repository.NewBranchDayRepo(db),
		ProductDay:    repository.NewProductDayRepo(db),
		PlatformDay:   repository.NewPlatformDayRepo(db),
		OrderContext:  repository.NewOrderContextRepo(db),
		EventIDs:      repository.NewEventIDsRepo(db),
	}
}
