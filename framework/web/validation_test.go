package web_test

import (
	"context"
	"testing"

	"github.com/NeftaliAcosta/springo/framework/ioc"
	"github.com/NeftaliAcosta/springo/framework/web"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Test DTOs ────────────────────────────────────────────────────────────────

type createUserDTO struct {
	Username string `json:"username" validate:"required,min=3"`
	Email    string `json:"email"    validate:"required,email" groups:"OnCreate"`
	Password string `json:"password" validate:"required,min=8"  groups:"OnCreate"`
}

type updateUserDTO struct {
	Username string `json:"username" validate:"required,min=3"`
	Email    string `json:"email"    validate:"required,email" groups:"OnUpdate"`
	// Password intentionally absent on update
}

// ─── Validate (no groups) — legacy behavior ────────────────────────────────

func TestValidate_NoGroups_AllFieldsValidated(t *testing.T) {
	dto := createUserDTO{Username: "", Email: "bad", Password: ""}
	err := web.Validate(dto)
	require.Error(t, err)
}

func TestValidate_NoGroups_ValidStruct_NoError(t *testing.T) {
	dto := createUserDTO{Username: "alice", Email: "alice@example.com", Password: "secret1234"}
	err := web.Validate(dto)
	require.NoError(t, err)
}

// ─── Validate with OnCreate group ─────────────────────────────────────────

func TestValidate_OnCreate_MissingPassword_Fails(t *testing.T) {
	dto := createUserDTO{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "", // required in OnCreate group
	}
	err := web.Validate(dto, web.OnCreate{})
	require.Error(t, err)
}

func TestValidate_OnCreate_ValidData_Passes(t *testing.T) {
	dto := createUserDTO{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "secret1234",
	}
	err := web.Validate(dto, web.OnCreate{})
	require.NoError(t, err)
}

func TestValidate_OnCreate_MissingUsername_Fails(t *testing.T) {
	// Username has no groups tag → always validated
	dto := createUserDTO{
		Username: "", // no groups tag → always required
		Email:    "alice@example.com",
		Password: "secret1234",
	}
	err := web.Validate(dto, web.OnCreate{})
	require.Error(t, err)
}

// ─── Validate with OnUpdate group ─────────────────────────────────────────

func TestValidate_OnUpdate_PasswordNotRequired(t *testing.T) {
	// updateUserDTO has no Password field
	dto := updateUserDTO{
		Username: "alice",
		Email:    "alice@example.com",
	}
	err := web.Validate(dto, web.OnUpdate{})
	require.NoError(t, err)
}

func TestValidate_OnUpdate_EmailGroupSkipped_WhenGroupIsOnCreate(t *testing.T) {
	// email has groups:"OnUpdate" — when group is OnCreate, email constraint skipped
	dto := updateUserDTO{
		Username: "alice",
		Email:    "not-an-email", // invalid, but group=OnCreate so skipped
	}
	err := web.Validate(dto, web.OnCreate{})
	// Email field's group is OnUpdate — OnCreate doesn't include it → no error on email
	require.NoError(t, err)
}

func TestValidate_OnUpdate_BadEmail_Fails(t *testing.T) {
	dto := updateUserDTO{
		Username: "alice",
		Email:    "not-an-email",
	}
	err := web.Validate(dto, web.OnUpdate{})
	require.Error(t, err)
}

// ─── TranslateValidationErrors ────────────────────────────────────────────

func TestTranslateValidationErrors_ReturnsHumanReadable(t *testing.T) {
	dto := createUserDTO{Username: "al", Email: "bad", Password: "short"}
	err := web.Validate(dto)
	require.Error(t, err)

	var validationErrs validator.ValidationErrors
	require.ErrorAs(t, err, &validationErrs)

	msgs := web.TranslateValidationErrors(validationErrs)
	assert.NotEmpty(t, msgs)
	// Each message should be human-readable, not raw tag names
	for field, msg := range msgs {
		assert.NotEmpty(t, field)
		assert.NotEmpty(t, msg)
		t.Logf("field=%s msg=%s", field, msg)
	}
}

func TestTranslateValidationErrorsCtx_WithMessageSource(t *testing.T) {
	ms := web.NewMessageSource("en")
	// Pre-load translations via reflection or helper if needed, or register bean
	ioc.GetContainer().RegisterBean("messageSource", ms)
	defer func() {
		// Clean up bean if necessary
	}()

	dto := createUserDTO{Username: "al", Email: "bad", Password: "short"}
	err := web.Validate(dto)
	require.Error(t, err)

	var validationErrs validator.ValidationErrors
	require.ErrorAs(t, err, &validationErrs)

	ctx := context.WithValue(context.Background(), web.LocaleContextKey, "es")
	msgs := web.TranslateValidationErrorsCtx(ctx, validationErrs)
	assert.NotEmpty(t, msgs)
	assert.Contains(t, msgs, "username")
}

// ─── Custom Validator Registration ────────────────────────────────────────

type mockStringValidator struct {
	shouldFail bool
	message    string
}

func (m *mockStringValidator) IsValid(value string, ctx context.Context) (bool, string) {
	if m.shouldFail {
		return false, m.message
	}
	return true, ""
}

type dtoWithCustomTag struct {
	Code string `json:"code" validate:"required,no_bad_words"`
}

func TestRegisterValidator_CustomTag_Passes(t *testing.T) {
	err := web.RegisterValidator("no_bad_words", &mockStringValidator{shouldFail: false})
	if err != nil {
		t.Logf("Validator already registered (expected in test suite): %v", err)
	}

	dto := dtoWithCustomTag{Code: "hello"}
	err = web.Validate(dto)
	require.NoError(t, err)
}

func TestRegisterValidator_DuplicateTag_ReturnsError(t *testing.T) {
	// First registration (may already exist from previous test)
	_ = web.RegisterValidator("no_bad_words", &mockStringValidator{})

	// Second registration must fail
	err := web.RegisterValidator("no_bad_words", &mockStringValidator{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}
