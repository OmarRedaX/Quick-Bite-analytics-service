package dto

import (
	"net/http"
	"strconv"

	apperror "analytics-service/lib/errors"
)

// BranchDaysQuery is the validated input for
// GET /branches/:branchId/days?from=&to=.
type BranchDaysQuery struct {
	BranchID int64  `validate:"required,gt=0"`
	From     string `validate:"required,datetime=2006-01-02"`
	To       string `validate:"required,datetime=2006-01-02"`
}

func ParseBranchDaysQuery(r *http.Request, branchIDParam string) (BranchDaysQuery, error) {
	id, err := strconv.ParseInt(branchIDParam, 10, 64)
	if err != nil {
		id = 0
	}

	q := BranchDaysQuery{
		BranchID: id,
		From:     r.URL.Query().Get("from"),
		To:       r.URL.Query().Get("to"),
	}
	if err := validate.Struct(q); err != nil {
		return BranchDaysQuery{}, apperror.FromValidation(err)
	}
	return q, nil
}
