package errors

// StatusCoder interface identifies errors that carry an HTTP status code
type StatusCoder interface {
	error
	StatusCode() int
	ErrorCode() string
	ErrorDetails() any
}

// BusinessError is the base for all domain/business logic errors (equivalent to BusinessException)
type BusinessError struct {
	Message string
	Code    string
	Details any
}

func (e *BusinessError) Error() string {
	return e.Message
}

func (e *BusinessError) StatusCode() int {
	return 422
}

func (e *BusinessError) ErrorCode() string {
	return e.Code
}

func (e *BusinessError) ErrorDetails() any {
	return e.Details
}

// --- Client Errors (4xx) ---

type BadRequestError struct{ BusinessError }

func (e *BadRequestError) StatusCode() int { return 400 }

// BadRequest creates a new 400 error
func BadRequest(message, code string) *BadRequestError {
	return &BadRequestError{BusinessError{Message: message, Code: code}}
}

type UnauthorizedError struct{ BusinessError }

func (e *UnauthorizedError) StatusCode() int { return 401 }

// Unauthorized creates a new 401 error
func Unauthorized(message, code string) *UnauthorizedError {
	return &UnauthorizedError{BusinessError{Message: message, Code: code}}
}

type ForbiddenError struct{ BusinessError }

func (e *ForbiddenError) StatusCode() int { return 403 }

// Forbidden creates a new 403 error
func Forbidden(message, code string) *ForbiddenError {
	return &ForbiddenError{BusinessError{Message: message, Code: code}}
}

type ResourceNotFoundError struct{ BusinessError }

func (e *ResourceNotFoundError) StatusCode() int { return 404 }

// NotFound creates a new 404 error
func NotFound(message, code string) *ResourceNotFoundError {
	return &ResourceNotFoundError{BusinessError{Message: message, Code: code}}
}

type ConflictError struct{ BusinessError }

func (e *ConflictError) StatusCode() int { return 409 }

// Conflict creates a new 409 error
func Conflict(message, code string) *ConflictError {
	return &ConflictError{BusinessError{Message: message, Code: code}}
}

type PayloadTooLargeError struct{ BusinessError }

func (e *PayloadTooLargeError) StatusCode() int { return 413 }

// PayloadTooLarge creates a new 413 error
func PayloadTooLarge(message, code string) *PayloadTooLargeError {
	return &PayloadTooLargeError{BusinessError{Message: message, Code: code}}
}

type TooManyRequestsError struct{ BusinessError }

func (e *TooManyRequestsError) StatusCode() int { return 429 }

// TooManyRequests creates a new 429 error
func TooManyRequests(message, code string) *TooManyRequestsError {
	return &TooManyRequestsError{BusinessError{Message: message, Code: code}}
}

// --- Server Errors (5xx) ---

type InternalServerError struct{ BusinessError }

func (e *InternalServerError) StatusCode() int { return 500 }

// InternalServer creates a new 500 error
func InternalServer(message, code string) *InternalServerError {
	return &InternalServerError{BusinessError{Message: message, Code: code}}
}

type NotImplementedError struct{ BusinessError }

func (e *NotImplementedError) StatusCode() int { return 501 }

// NotImplemented creates a new 501 error
func NotImplemented(message, code string) *NotImplementedError {
	return &NotImplementedError{BusinessError{Message: message, Code: code}}
}

type ServiceUnavailableError struct{ BusinessError }

func (e *ServiceUnavailableError) StatusCode() int { return 503 }

// ServiceUnavailable creates a new 503 error
func ServiceUnavailable(message, code string) *ServiceUnavailableError {
	return &ServiceUnavailableError{BusinessError{Message: message, Code: code}}
}

type GatewayTimeoutError struct{ BusinessError }

func (e *GatewayTimeoutError) StatusCode() int { return 504 }

// GatewayTimeout creates a new 504 error
func GatewayTimeout(message, code string) *GatewayTimeoutError {
	return &GatewayTimeoutError{BusinessError{Message: message, Code: code}}
}

// Business creates a new 422 error
func Business(message, code string) *BusinessError {
	return &BusinessError{Message: message, Code: code}
}

// DetailedBusiness creates a new 422 error with extra details
func DetailedBusiness(message, code string, details any) *BusinessError {
	return &BusinessError{Message: message, Code: code, Details: details}
}

// New is a generic factory for any error type (Future proof)
func New(status int, message, code string) error {
	base := BusinessError{Message: message, Code: code}
	switch status {
	case 400:
		return &BadRequestError{base}
	case 401:
		return &UnauthorizedError{base}
	case 403:
		return &ForbiddenError{base}
	case 404:
		return &ResourceNotFoundError{base}
	case 409:
		return &ConflictError{base}
	case 500:
		return &InternalServerError{base}
	default:
		return &base
	}
}
