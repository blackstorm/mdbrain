package handlers

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"mdbrain.dev/internal/config"
	"mdbrain.dev/internal/infra/repository"
)

type InternalHandler struct {
	cfg  *config.Config
	repo *repository.Repository
}

func NewInternalHandler(cfg *config.Config, repo *repository.Repository) *InternalHandler {
	return &InternalHandler{cfg: cfg, repo: repo}
}

func (h *InternalHandler) Health(c *echo.Context) error {
	if c.Request().Header.Get("X-Health-Token") != h.cfg.HealthToken && c.QueryParam("token") != h.cfg.HealthToken {
		return c.String(http.StatusUnauthorized, "unauthorized")
	}
	return c.String(http.StatusOK, "ok")
}

func (h *InternalHandler) Robots(c *echo.Context) error {
	c.Response().Header().Set("Content-Type", "text/plain; charset=utf-8")
	return c.String(http.StatusOK, "User-agent: *\nDisallow: /\n")
}

func (h *InternalHandler) DomainCheck(c *echo.Context) error {
	vault, err := h.repo.GetVaultByDomain(c.Request().Context(), c.QueryParam("domain"))
	if err != nil || vault == nil {
		return c.String(http.StatusNotFound, "not found")
	}
	return c.String(http.StatusOK, "ok")
}
