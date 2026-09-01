package dto

import (
	"net/http"
	"strconv"

	apperror "analytics-service/lib/errors"
)

// ProductDaysQuery is the validated input for
// GET /branches/:branchId/products/:productId/days?from=&to=.
type ProductDaysQuery struct {
	BranchID  int64  `validate:"required,gt=0"`
	ProductID int64  `validate:"required,gt=0"`
	From      string `validate:"required,datetime=2006-01-02"`
	To        string `validate:"required,datetime=2006-01-02"`
}

func ParseProductDaysQuery(r *http.Request, branchIDParam, productIDParam string) (ProductDaysQuery, error) {
	branchID, err := strconv.ParseInt(branchIDParam, 10, 64)
	if err != nil {
		branchID = 0
	}
	productID, err := strconv.ParseInt(productIDParam, 10, 64)
	if err != nil {
		productID = 0
	}

	q := ProductDaysQuery{
		BranchID:  branchID,
		ProductID: productID,
		From:      r.URL.Query().Get("from"),
		To:        r.URL.Query().Get("to"),
	}
	if err := validate.Struct(q); err != nil {
		return ProductDaysQuery{}, apperror.FromValidation(err)
	}
	return q, nil
}
