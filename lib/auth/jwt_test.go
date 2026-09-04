package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Colocated unit test — same package, so it can construct jwtClaims
// directly to mint tokens for VerifyAccessToken to check. This is a
// deliberate exception to "this service never signs tokens" (see
// tests/integration/testutil.MintAccessToken's doc comment): unlike the
// integration helper, this file only needs the wire shape to exercise
// VerifyAccessToken's failure modes, not a full router.

const testSecret = "unit-test-secret"

func signToken(t *testing.T, secret string, method jwt.SigningMethod, claims jwtClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(method, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func TestVerifyAccessToken_ValidRoundTrip(t *testing.T) {
	restaurantID := int64(42)
	claims := jwtClaims{
		UserID:         1,
		Role:           "restaurant_user",
		Email:          "owner@quickbite.test",
		RestaurantID:   &restaurantID,
		RestaurantRole: "owner",
		BranchIDs:      []int64{1, 2},
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := signToken(t, testSecret, jwt.SigningMethodHS256, claims)

	got, err := VerifyAccessToken(token, testSecret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.UserID != 1 || got.Role != "restaurant_user" || got.Email != "owner@quickbite.test" || got.RestaurantRole != "owner" {
		t.Fatalf("expected claims round-tripped, got %+v", got)
	}
	if got.RestaurantID == nil || *got.RestaurantID != restaurantID {
		t.Fatalf("expected restaurantId round-tripped, got %v", got.RestaurantID)
	}
	if len(got.BranchIDs) != 2 || got.BranchIDs[0] != 1 || got.BranchIDs[1] != 2 {
		t.Fatalf("expected branchIds round-tripped, got %v", got.BranchIDs)
	}
}

func TestVerifyAccessToken_WrongSecret(t *testing.T) {
	claims := jwtClaims{
		UserID: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := signToken(t, testSecret, jwt.SigningMethodHS256, claims)

	if _, err := VerifyAccessToken(token, "a-different-secret"); err == nil {
		t.Fatal("expected an error when the verifying secret doesn't match the signing secret")
	}
}

func TestVerifyAccessToken_Expired(t *testing.T) {
	claims := jwtClaims{
		UserID: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	}
	token := signToken(t, testSecret, jwt.SigningMethodHS256, claims)

	if _, err := VerifyAccessToken(token, testSecret); err == nil {
		t.Fatal("expected an error for a token with exp in the past")
	}
}

func TestVerifyAccessToken_WrongSigningMethodRejected(t *testing.T) {
	// jwt.WithValidMethods([]string{"HS256"}) must reject a token signed
	// with "none" — a classic JWT vulnerability if not explicitly checked.
	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwtClaims{UserID: 1})
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign unsafe none-alg token: %v", err)
	}

	if _, err := VerifyAccessToken(signed, testSecret); err == nil {
		t.Fatal("expected the alg=none token to be rejected by WithValidMethods([]string{\"HS256\"})")
	}
}

func TestVerifyAccessToken_MalformedToken(t *testing.T) {
	if _, err := VerifyAccessToken("not-a-jwt-at-all", testSecret); err == nil {
		t.Fatal("expected an error for a malformed token string")
	}
}
