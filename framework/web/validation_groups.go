package web

// ValidationGroup is a marker interface for validation groups.
// Equivalent to Spring Boot's validation group interfaces.
// Embed or use as struct tag groups in validate:"..." constraints.
type ValidationGroup interface{}

// Built-in validation groups provided by the framework.
// Use in Dispatch(fn, WithValidationGroup(web.OnCreate{}))

// OnCreate marks validation constraints that apply only during resource creation (POST).
type OnCreate struct{}

// OnUpdate marks validation constraints that apply only during resource updates (PUT/PATCH).
type OnUpdate struct{}

// OnDelete marks validation constraints that apply only during deletion.
type OnDelete struct{}
