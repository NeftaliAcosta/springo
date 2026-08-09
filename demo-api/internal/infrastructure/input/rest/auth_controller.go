package rest

import (
	"context"
	"fmt"
	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/dtos/request"
	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/dtos/response"
	"github.com/NeftaliAcosta/springo/framework/config"
	frameworkSecurity "github.com/NeftaliAcosta/springo/framework/security"
	"github.com/NeftaliAcosta/springo/framework/web"

	"github.com/go-chi/chi/v5"
)

type AuthController struct {
	jwtProvider *frameworkSecurity.JwtProvider
}

func init() {
	web.Register(func(r chi.Router) {
		jwtProps := config.Get[frameworkSecurity.JwtProperties]()
		if jwtProps == nil {
			// Fallback if config is missing
			jwtProps = &frameworkSecurity.JwtProperties{Secret: "default-secret", Expiration: 60}
		}
		provider := frameworkSecurity.NewJwtProvider(jwtProps.Secret, jwtProps.Expiration)
		c := &AuthController{jwtProvider: provider}

		r.Post("/auth/login", web.Dispatch(c.login))
	})
}

// @Summary Authenticate and get token
// @Description Login with username and password to receive a JWT token
// @Tags Authentication
// @Accept json
// @Produce json
// @Param login body request.LoginRequestDTO true "Credentials"
// @Success 200 {object} web.ApiResponse[response.LoginResponseDTO]
// @Failure 401 {object} web.ApiResponse[any]
// @Router /auth/login [post]
func (c *AuthController) login(ctx context.Context, req request.LoginRequestDTO) (any, error) {
	// Simple mock authentication
	if req.Username == "admin" && req.Password == "password" {
		roles := []string{"ADMIN", "USER"}
		token, err := c.jwtProvider.GenerateToken(req.Username, roles)
		if err != nil {
			return nil, err
		}
		return response.LoginResponseDTO{Token: token, Roles: roles}, nil
	} else if req.Username == "user" && req.Password == "password" {
		roles := []string{"USER"}
		token, err := c.jwtProvider.GenerateToken(req.Username, roles)
		if err != nil {
			return nil, err
		}
		return response.LoginResponseDTO{Token: token, Roles: roles}, nil
	}

	return nil, fmt.Errorf("invalid credentials")
}
