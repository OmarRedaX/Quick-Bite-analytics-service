// Package auth verifies the platform's shared access token (same
// ACCESS_SECRET/HS256/claims shape as core-service and order-service) and
// exposes HTTP middleware that turns a valid token into appcontext.Claims.
package auth

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"

	"analytics-service/lib/appcontext"
)

// jwtClaims mirrors order-service's JWTPayload (src/lib/auth/jwt.ts).
// json tags match the wire shape core/order already sign — this service
// only ever verifies, never signs.
type jwtClaims struct {
	UserID         int64   `json:"userId"`
	Role           string  `json:"role"`
	Email          string  `json:"email"`
	RestaurantID   *int64  `json:"restaurantId,omitempty"`
	RestaurantRole string  `json:"restaurantRole,omitempty"`
	BranchIDs      []int64 `json:"branchIds,omitempty"`
	jwt.RegisteredClaims
}

var errInvalidToken = errors.New("invalid or expired access token")

// VerifyAccessToken parses and validates tokenString against secret
// (HS256 only — mirrors jsonwebtoken's verify(token, ACCESS_SECRET) on the
// Node side). Returns appcontext.Claims ready to stash on the request
// context.
func VerifyAccessToken(tokenString, secret string) (appcontext.Claims, error) {
	var claims jwtClaims
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(t *jwt.Token) (any, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !token.Valid {
		return appcontext.Claims{}, errInvalidToken
	}

	return appcontext.Claims{
		UserID:         claims.UserID,
		Role:           claims.Role,
		Email:          claims.Email,
		RestaurantID:   claims.RestaurantID,
		RestaurantRole: claims.RestaurantRole,
		BranchIDs:      claims.BranchIDs,
	}, nil
}
