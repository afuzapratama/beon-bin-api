package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/beon/bin-api/internal/model"
	"github.com/beon/bin-api/internal/service"
	"github.com/beon/bin-api/pkg/validator"
	"github.com/gin-gonic/gin"
)

const poweredBy = "BEON API"

type BINHandler struct {
	svc binService
}

type binService interface {
	Lookup(c context.Context, bin string) (*model.BINResponse, error)
	Stats(c context.Context) (int64, error)
}

func NewBINHandler(svc binService) *BINHandler {
	return &BINHandler{svc: svc}
}

// Lookup handles GET /api/v1/bin/:number
func (h *BINHandler) Lookup(c *gin.Context) {
	bin := c.Param("number")

	if !validator.IsValidBIN(bin) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success":    false,
			"error":      "BIN must be exactly 6 or 8 ASCII digits",
			"code":       400,
			"powered_by": poweredBy,
		})
		return
	}

	result, err := h.svc.Lookup(c.Request.Context(), bin)
	if errors.Is(err, service.ErrEnrichmentRateLimited) {
		c.Header("Retry-After", "60")
		c.JSON(http.StatusTooManyRequests, gin.H{
			"success":    false,
			"error":      "BIN enrichment providers are rate limited",
			"code":       429,
			"powered_by": poweredBy,
		})
		return
	}
	if errors.Is(err, service.ErrEnrichmentUnavailable) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success":    false,
			"error":      "BIN enrichment temporarily unavailable",
			"code":       503,
			"powered_by": poweredBy,
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":    false,
			"error":      "Internal server error",
			"code":       500,
			"powered_by": poweredBy,
		})
		return
	}
	if result == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success":    false,
			"error":      "BIN not found",
			"code":       404,
			"powered_by": poweredBy,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       result,
		"powered_by": poweredBy,
	})
}

// Stats handles GET /api/v1/stats
func (h *BINHandler) Stats(c *gin.Context) {
	count, err := h.svc.Stats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success":    false,
			"error":      "Failed to get stats",
			"code":       500,
			"powered_by": poweredBy,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"total_bins": count,
		"powered_by": poweredBy,
	})
}
