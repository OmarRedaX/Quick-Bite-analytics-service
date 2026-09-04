package apperror

import (
	"errors"
	"net/http"
	"testing"

	"github.com/go-playground/validator/v10"
)

// Colocated unit test — same package, exercising AppError's Code()/
// StatusCode()/Error()/Unwrap() contract and FromValidation's mapping from
// go-playground/validator errors to a stable VALIDATION_ERROR AppError.

func TestAppError_CodeStatusCodeErrorContract(t *testing.T) {
	err := New("SOME_CODE", http.StatusBadRequest, "human readable message")

	if err.Code() != "SOME_CODE" {
		t.Fatalf("expected code SOME_CODE, got %s", err.Code())
	}
	if err.StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", err.StatusCode())
	}
	if err.Error() != "human readable message" {
		t.Fatalf("expected Error() to return the message, got %s", err.Error())
	}
	if err.Unwrap() != nil {
		t.Fatalf("expected Unwrap() nil for an AppError with no cause, got %v", err.Unwrap())
	}
}

func TestAppError_WithCause_UnwrapsToOriginalError(t *testing.T) {
	cause := errors.New("underlying failure")
	err := WithCause(cause, "WRAPPED", http.StatusInternalServerError, "something broke")

	if !errors.Is(err, cause) {
		t.Fatal("expected errors.Is to find the wrapped cause via Unwrap()")
	}
	if err.Error() != "something broke" {
		t.Fatalf("expected Error() to return the AppError's own message, not the cause's, got %s", err.Error())
	}
}

type testStruct struct {
	Name string `validate:"required"`
	Age  int    `validate:"gte=0"`
}

func TestFromValidation_MapsValidatorErrorsTo400(t *testing.T) {
	v := validator.New()
	err := v.Struct(testStruct{Name: "", Age: -1})
	if err == nil {
		t.Fatal("expected the validator to reject an empty Name and negative Age")
	}

	appErr := FromValidation(err)
	if appErr.Code() != "VALIDATION_ERROR" {
		t.Fatalf("expected code VALIDATION_ERROR, got %s", appErr.Code())
	}
	if appErr.StatusCode() != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", appErr.StatusCode())
	}
	if appErr.Error() == "" {
		t.Fatal("expected a non-empty message summarizing the failing fields")
	}
}

func TestFromValidation_NonValidatorError_FallsBackToRawMessage(t *testing.T) {
	appErr := FromValidation(errors.New("not a validator error"))

	if appErr.Code() != "VALIDATION_ERROR" {
		t.Fatalf("expected code VALIDATION_ERROR even for a non-validator error, got %s", appErr.Code())
	}
	if appErr.Error() != "not a validator error" {
		t.Fatalf("expected the raw error message passed through, got %s", appErr.Error())
	}
}
