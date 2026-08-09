package rest

import (
	"context"
	"github.com/NeftaliAcosta/springo/demo-api/internal/application/port/output"
	"github.com/NeftaliAcosta/springo/demo-api/internal/domain/port/in"
	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/dtos/request"
	_ "github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/dtos/response"
	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/validators"
	"github.com/NeftaliAcosta/springo/framework/web"
	"log"

	"github.com/go-chi/chi/v5"
)

// UserController handles web requests (Primary Adapter)
type UserController struct {
	userUseCase in.UserUseCase
}

// init registers the controller and custom validators to the framework
func init() {
	web.Register(func(r chi.Router) {
		service := web.GetService[in.UserUseCase]("UserService")
		userRepo := web.GetService[output.UserPersistencePort]("UserRepository")
		NewUserController(r, service, userRepo)
	})
}

// NewUserController initializes the controller and registers custom validators
func NewUserController(r chi.Router, s in.UserUseCase, userRepo output.UserPersistencePort) {
	c := &UserController{userUseCase: s}

	// Register custom validators (requires IoC to be initialized)
	if err := web.RegisterValidator("unique_email", &validators.UniqueEmailValidator{
		UserRepo: userRepo,
	}); err != nil {
		log.Printf("⚠️ [Validation] %v", err)
	}

	r.Route("/users", func(r chi.Router) {
		// POST: create — OnCreate group → validates password + unique_email
		r.Post("/", web.Dispatch(c.create,
			web.WithRoles("ADMIN"),
			web.WithValidationGroup(web.OnCreate{}),
		))

		// GET /: list all — no DTO validation needed
		r.Get("/", web.Dispatch(c.list,
			web.WithRoles("ADMIN"),
		))

		// GET /error: demo error endpoint — no body validation
		r.Get("/error", web.Dispatch(c.errorDemo))

		// GET /{id}: get by ID — path param validated
		r.Get("/{id}", web.Dispatch(c.get))

		// POST /complex: demo @Transactional + OnCreate group
		r.Post("/complex", web.DispatchTx(c.complexDemo,
			web.WithValidationGroup(web.OnCreate{}),
		))

		// PUT /{id}: update — OnUpdate group → no password required
		r.Put("/{id}", web.Dispatch(c.update,
			web.WithRoles("ADMIN", "MANAGER"),
			web.WithValidationGroup(web.OnUpdate{}),
		))
	})
}

// @Summary Create a new user
// @Tags Users
// @Security BearerAuth
// @Success 201 {object} web.ApiResponse[response.UserResponseDTO]
// @Router /users [post]
func (c *UserController) create(ctx context.Context, req request.UserRequestDTO) (any, error) {
	return c.userUseCase.CreateUser(ctx, req)
}

// @Summary List all users
// @Tags Users
// @Security BearerAuth
// @Success 200 {object} web.ApiResponse[[]response.UserResponseDTO]
// @Router /users [get]
func (c *UserController) list(ctx context.Context, traceID web.TraceID) (any, error) {
	_ = traceID
	return c.userUseCase.GetAllUsers(ctx)
}

// @Summary Trigger a controlled error
// @Tags Users
// @Security BearerAuth
// @Failure 400 {object} web.ApiResponse[any]
// @Router /users/error [get]
func (c *UserController) errorDemo(ctx context.Context) (any, error) {
	return nil, c.userUseCase.TriggerBusinessError(ctx)
}

// @Summary Get user by ID
// @Tags Users
// @Security BearerAuth
// @Success 200 {object} web.ApiResponse[response.UserResponseDTO]
// @Router /users/{id} [get]
func (c *UserController) get(ctx context.Context, req request.UserDetailRequestDTO) (any, error) {
	return c.userUseCase.GetUserByID(ctx, req.ID)
}

// @Summary Update user
// @Tags Users
// @Security BearerAuth
// @Success 200 {object} web.ApiResponse[response.UserResponseDTO]
// @Router /users/{id} [put]
func (c *UserController) update(ctx context.Context, req request.UserUpdateRequestDTO) (any, error) {
	return c.userUseCase.UpdateUser(ctx, req)
}

// @Summary Demonstrate transactional propagation and DispatchTx usage
// @Tags Users
// @Security BearerAuth
// @Router /users/complex [post]
func (c *UserController) complexDemo(ctx context.Context, req request.UserRequestDTO) (any, error) {
	err := c.userUseCase.ComplexRegistration(ctx, req)
	if err != nil {
		return nil, err
	}
	return "User complex registration completed successfully", nil
}
