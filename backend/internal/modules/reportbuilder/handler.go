package reportbuilder

import (
	"net/http"
	"strconv"
	"strings"

	"pharmacy-erp/backend/internal/modules/audit"
	"pharmacy-erp/backend/internal/platform"

	"github.com/labstack/echo/v4"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Catalog(c echo.Context) error {
	return platform.JSON(c, http.StatusOK, h.service.CatalogForUser(platform.CurrentUser(c)))
}

func (h *Handler) Execute(c echo.Context) error {
	var input ExecuteRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	user := platform.CurrentUser(c)
	result, err := h.service.Execute(c.Request().Context(), user, user.ID, input)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) List(c echo.Context) error {
	items, err := h.service.List(c.Request().Context(), platform.CurrentUser(c).ID, strings.TrimSpace(c.QueryParam("pin_target")))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Create(c echo.Context) error {
	var input SaveReportRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	result, err := h.service.Create(c.Request().Context(), platform.CurrentUser(c).ID, input, audit.MetaFromContext(c))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusCreated, result)
}

func (h *Handler) Get(c echo.Context) error {
	result, err := h.service.Get(c.Request().Context(), platform.CurrentUser(c).ID, strings.TrimSpace(c.Param("reportID")))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) Update(c echo.Context) error {
	var input SaveReportRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	result, err := h.service.Update(c.Request().Context(), platform.CurrentUser(c).ID, strings.TrimSpace(c.Param("reportID")), input, audit.MetaFromContext(c))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) Delete(c echo.Context) error {
	if err := h.service.Delete(c.Request().Context(), platform.CurrentUser(c).ID, strings.TrimSpace(c.Param("reportID")), audit.MetaFromContext(c)); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "ลบรูปแบบรายงานแล้ว")
}

func (h *Handler) Pin(c echo.Context) error {
	var input PinRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	result, err := h.service.SetPinned(c.Request().Context(), platform.CurrentUser(c).ID, strings.TrimSpace(c.Param("reportID")), input.Pinned, input.PinTargetKey, audit.MetaFromContext(c))
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, result)
}

func (h *Handler) ReorderPins(c echo.Context) error {
	var input ReorderPinsRequest
	if err := c.Bind(&input); err != nil {
		return platform.HandleHTTPError(c, platform.NewError(http.StatusBadRequest, "invalid request body"))
	}
	if err := h.service.ReorderPins(c.Request().Context(), platform.CurrentUser(c).ID, input.PinTargetKey, input.ReportIDs, audit.MetaFromContext(c)); err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSONMessage(c, http.StatusOK, "บันทึกลำดับรายงานที่ปักหมุดแล้ว")
}

// FieldValues backs the filter builder's value picker.
func (h *Handler) FieldValues(c echo.Context) error {
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	values, err := h.service.FieldValues(
		c.Request().Context(),
		platform.CurrentUser(c),
		strings.TrimSpace(c.QueryParam("dataset")),
		strings.TrimSpace(c.QueryParam("field")),
		limit,
	)
	if err != nil {
		return platform.HandleHTTPError(c, err)
	}
	return platform.JSON(c, http.StatusOK, map[string]any{"items": values})
}
